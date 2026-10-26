# T1 — Power & network lifecycle (thiết kế)

Ngày: 2026-10-27. Phạm vi: gập máy/suspend/hibernate, mất mạng, đổi Wi-Fi/IP,
khôi phục sau khi daemon ngủ/chết, không mất/không trùng dữ liệu. Không gồm
tray/autostart (T2–T4). Nguồn: `docs/handoff/phase-12-spec.md` PHẦN T.

## 1. Audit hiện trạng (2026-10-27)

| Thành phần | Hiện có | Thiếu |
| --- | --- | --- |
| `internal/engine` | intent + `FindByIntent`, `ActiveJobs` đưa `executing` về `reconciling` khi khởi động lại, backoff mũ, `NotExecuted` (không gửi lại mù) | không biết máy/mạng đang ngủ; lỗi `unreachable` vẫn đốt `attempts` và vẫn `FailListing` ⇒ job kết thúc `partial` khi chỉ mất mạng |
| `internal/daemon` | vòng lặp engine/syncer/bot, `POST /v1/shutdown`, không đăng ký tín hiệu OS | không có trạng thái hoạt động (chạy/ngủ/đánh thức), không nghe logind/Power API, không heartbeat, không reset connection pool sau ngủ |
| `internal/state` | `settings(key,value)` KV (dùng cho default destination), `jobs/items/events` | không có `wal_checkpoint` lúc rảnh; không có hàm "nhả item không phạt attempt" |
| `internal/app.Status` | version, schema, ready, account, bot_state, counts | không có trạng thái mạng/ngủ/đánh thức |
| `internal/drive` | `KindUnreachable` + `NotExecuted`, `invalid_grant` → `ErrReconnectRequired` | — (đã đủ để phân loại) |
| `internal/config` | JSON config, `Validate()` | không có tùy chọn giữ máy thức |
| syncer | `ClaimDueSchedules` gom lần lỡ thành **một** run/collection; test `TestScheduledRunCatchesUpExactlyOnceAfterDowntime` | chưa có test bắc cầu từ phát hiện gap sang scheduler (T1 thêm ở mức lifecycle) |

Tray/`fyne.io/systray` đã xử lý `TaskbarCreated` (Explorer restart) — dùng cho T2,
không đụng ở T1.

## 2. Dependency

| Cần | Dùng | Chi phí | Quyết định |
| --- | --- | --- | --- |
| D-Bus (logind) trên Linux | `github.com/godbus/dbus/v5` | **đã có** trong `go.mod` (indirect, kéo bởi `fyne.io/systray`); thuần Go, 0 dependency phụ | dùng, chuyển thành direct require (không tải module mới) |
| Suspend/resume trên Windows | `PowerRegisterSuspendResumeNotification` (powrprof.dll) qua `golang.org/x/sys/windows` + `syscall.NewCallback` | `x/sys` **đã có** (indirect); API cần bind bằng `windows.NewLazySystemDLL` | dùng, không thêm module |
| Network change trên Windows | `NotifyIpInterfaceChange` | có sẵn trong `x/sys/windows` (đã kiểm `zsyscall_windows.go`) | dùng (chỉ để reset pool + wake) |
| Network change trên Linux | probe (không thêm NetworkManager D-Bus) | 0 | chọn probe: rẻ hơn, đủ tin cậy; logind chỉ cho suspend/resume |
| Darwin/khác | không có API rẻ | 0 | chỉ gap |

Không thêm module ngoài. `go mod tidy` chỉ chuyển `godbus` từ indirect → direct.

## 3. Kiến trúc

```
OS adapter (build tag)          power.Monitor (thuần Go)          daemon.lifecycle
┌───────────────────┐    events    ┌──────────────────┐  events    ┌──────────────────┐
│ logind PrepareFor ├─────────────► │ dedupe/flap      ├───────────►│ state machine    │
│ Sleep + delay lock │              │ gap ticker 5s    │            │ suspend/resume   │
│ PowerRegister… (Win)│             │ Suspend/Resume   │            │ checkpoint/probe │
└───────────────────┘              └──────────────────┘            └───────┬──────────┘
                                                     gate (online/offline)│  heartbeat (settings KV)
                                                     ┌────────────────────┴──────────┐
                                                     ▼                               ▼
                                             engine.Engine.Network            app.Status.Lifecycle
```

### 3.1 `internal/power` (mới, không phụ thuộc daemon/engine)

```go
type Kind int
const (Suspend; Resume; NetworkChanged; SessionLock; SessionUnlock; Shutdown)

type Event struct {
    Kind Kind
    At   time.Time
    Gap  time.Duration // chỉ với Resume
    Ack  func()        // nhả delay-inhibitor lock (idempotent, có thể nil)
}

type Source interface { Events() <-chan Event; Close() error }
```

- `Gap`: so `wall(t1)-wall(t0)` với `mono(t1)-mono(t0)`; chênh > `Threshold`
  (mặc định 30s) ⇒ `(gap, true)`. Chạy mỗi 5s trong `Monitor`; đây là **nền tảng
  bắt buộc**, hoạt động cả khi OS không có API hoặc sự kiện bị lỡ.
- `Monitor`: nhận event từ `Source` + ticker gap; chuẩn hóa:
  - `Suspend` chỉ phát khi đang online; `Resume` chỉ phát khi đang suspended;
  - gap khi đang online ⇒ phát `Resume{Gap: d}` (đã ngủ mà không thấy event);
  - `Resume` trùng trong cùng một cửa sổ (flap) bị bỏ; `NetworkChanged` luôn phát.
- `Gate`: cờ online/offline có `Wait(ctx)` broadcast (channel đóng khi đổi). Engine
  dùng để **không claim item** khi offline.
- `Prober`: `Until(ctx)` probe tuần tự (mutex = tối đa 1 probe), backoff mũ
  `Base=1s → Max=30s`, jitter ±20% (hàm ngẫu nhiên injectable).
- `WakeKeeper`: giữ máy thức (opt-in) — Linux logind `Inhibit("sleep:idle",…,"block")`,
  Windows `SetThreadExecutionState` trên một OS thread riêng, nơi khác no-op.

### 3.2 Adapter

**Linux** (`source_linux.go`, `//go:build linux`): system bus; match
`org.freedesktop.login1.Manager.PrepareForSleep`; `true` → cố lấy **delay
inhibitor** (`Inhibit("sleep","502Drive","Checkpoint before sleep","delay")`,
mode delay; logind cho tối đa `InhibitDelayMaxSec`, mặc định 5s), phát
`Suspend` với `Ack` đóng fd; `false` → `Resume`. Không có bus/logind ⇒
`Source` rỗng (chỉ gap). Không giữ lock quá 5s: goroutine `Ack` tự đóng sau
`delayMax` kể cả khi daemon chậm. Không ngăn sleep mặc định.

**Windows** (`source_windows.go`): `PowerRegisterSuspendResumeNotification`
(`DEVICE_NOTIFY_CALLBACK`) → `PBT_APMSUSPEND` = Suspend, `PBT_APMRESUMEAUTOMATIC`
/`PBT_APMRESUMESUSPEND` = Resume; `NotifyIpInterfaceChange` → NetworkChanged.
Đăng ký thất bại ⇒ chỉ gap (log warn). Callback chỉ đẩy vào channel (không chặn);
con trỏ callback/params được giữ sống trong struct toàn cục.

**Khác**: `source_other.go` trả source rỗng; `Monitor` vẫn phát hiện gap.

### 3.3 `internal/daemon/lifecycle.go`

Trạng thái: `running → suspending → sleeping → resuming → running`, cộng
`reconnect_required` (token `invalid_grant`).

- **Startup**: đọc `power.heartbeat` (settings). `now-hb > 30s` ⇒ coi như vừa
  có gap: `resuming`, gate **offline**, chạy probe; ngược lại `running`, gate
  online. Luôn reset idle connections + `eng.Wake()`.
- **Suspend**: `suspending` (admission từ chối plan/job mới bằng 503
  `suspending`), gate offline (worker không claim thêm; **không hủy** request
  đang bay), `PRAGMA wal_checkpoint(PASSIVE)` với ngân sách 2s, ghi heartbeat,
  `sleeping`. Kết thúc bằng `Ack()` (nhả delay lock) sau checkpoint hoặc khi
  2s hết.
- **Resume(gap)**: `resuming`; (a) `store.Check` (DB), (b) cho phép admission,
  (c) `resetConnections()` (idle pool), (d) `Prober.Until` probe nhẹ
  (`provider.Account`) với backoff; thành công ⇒ gate online, `eng.Wake()`,
  `running`; lỗi auth/`ErrReconnectRequired` ⇒ `reconnect_required` và probe
  thưa 5 phút (không retry dồn dập). Ghi `power.last_gap`.
- **Keep awake** (opt-in `keep_awake_while_copying`): tick 10s, khi
  `running_jobs > 0` thì `WakeKeeper.Hold()` ngược lại `Release()`.
- Mọi chuyển trạng thái ghi log + cập nhật `app.Status.Lifecycle`.

### 3.4 Engine (thay đổi nhỏ, giữ bất biến)

- `Engine.Network NetworkGate` (`Online() bool`, `Wait(ctx) bool`) — interface
  khai báo trong `engine`, `*power.Gate` thỏa mãn; engine không import `power`.
- Worker: trước `ClaimItem` gọi `waitNetwork` (dừng nhận việc mới, không claim).
- `process`: lỗi `KindUnreachable` (chắc chắn chưa gửi) ⇒
  `ReleaseItemNoPenalty` (pending, `next_attempt_at=0`, **giảm** `attempts` vì
  `ClaimItem` đã tăng) rồi thôi; không `failed`.
- `listPage`: `KindUnreachable` ⇒ chờ mạng rồi đọc lại **cùng trang**, không
  `FailListing`, không tiêu `maxTransientAttempts`.
- Ghi có kết quả không rõ (`KindUnavailable`/timeout) vẫn đi `reconcile`
  (`FindByIntent`) trước khi gửi lại — không đổi.

### 3.5 HTTP / status

- `GET /v1/status` thêm `lifecycle {state, online, since, last_gap_seconds,
  reconnect_required}`; `state ∈ running|suspending|sleeping|resuming|
  reconnect_required`. OpenAPI + conformance cập nhật.
- `POST /v1/plans`, `POST /v1/jobs`, `POST /v1/collections/{id}/run`,
  `PUT /v1/collections/{id}/sync` trả 503 `suspending` khi đang `suspending`
  (cửa sổ ≤2s). Không đổi mã lỗi khác.

## 4. Chế độ lỗi & quyết định

| Tình huống | Hành vi | Vì sao an toàn |
| --- | --- | --- |
| Ngủ giữa `CopyFile` (mất phản hồi) | item `executing`; sau resume request lỗi ⇒ nếu `NotExecuted` nhả không phạt, nếu không rõ ⇒ `reconciling` + `FindByIntent` | không gửi lại mù; intent là duy nhất theo (collection, source, parent, kind, generation) |
| Sự kiện OS bị lỡ | gap ticker phát `Resume(gap)` | không phụ thuộc API OS |
| Resume dồn dập (flap) | `Monitor` bỏ Resume trùng; `Prober` mutex | không bão probe |
| Mất mạng kéo dài | gate offline; job `running`, item pending; không tăng attempt | attempt chỉ dùng cho lỗi thật |
| `invalid_grant` | `reconnect_required`, probe 5 phút/lần | không retry vô hạn, hiện hành động rõ |
| Đổi Wi-Fi/IP | probe lỗi ⇒ gate offline ⇒ reset pool + wake | không dùng connection chết |
| Daemon chết trong lúc ngủ | startup: heartbeat cũ ⇒ resuming + probe; `ActiveJobs` đưa `executing`→`reconciling` | reconcile trước khi gửi lại |
| Ngủ khi đang giữ keep-awake | block inhibitor bị OS bỏ khi hibernate | chỉ là tối ưu, không phải bất biến |

## 5. Không làm trong T1

- Không ngăn sleep mặc định; không đổi lịch/`schedule`; không thêm UI tray
  (T4), không đổi autostart (T3), không thêm `auto` language (T5).
- Không theo dõi logind `SessionLock` trên Windows (chỉ map khi adapter có);
  Linux phát `SessionLock/Unlock` nhưng daemon chỉ log (không đổi trạng thái job).
- Không detect Wi-Fi trên Linux bằng NetworkManager (chọn probe).

## 6. Kiểm chứng (mô phỏng)

`internal/power`: gap detector (30s/8h/clock step), monitor flap, gate Wait,
prober backoff + single-flight + jitter seed.
`internal/daemon`: suspend giữa copy (fake provider giữ request rồi lỗi) → resume
→ không trùng; resume flap → một probe; `invalid_grant` → `reconnect_required`;
đổi mạng → `resetConnections` được gọi; heartbeat cũ lúc startup; admission 503
trong `suspending`; keep-awake bật/tắt theo job.
`internal/engine`: `TestOfflineDoesNotBurnAttempts`, `TestListPageWaitsForNetwork`,
ma trận crash ngẫu nhiên có seed (`-short` bỏ qua).
`internal/httpapi`: status có `lifecycle`, conformance OpenAPI.

Cột **OS thật** trong `docs/acceptance.md` mặc định `chưa chạy`; checklist tay ở
`docs/runbooks.md#kiểm-tra-laptop-thật`.

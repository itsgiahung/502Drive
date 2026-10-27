# H3 — CLI nhanh

Ngày: 2026-10-27. Không đổi stdout/JSON/exit code đã công bố.

## Lệnh và quy ước

- `502drive LINK [LINK...] [flags]` = `502drive copy …`. `Execute` chèn `copy`
  khi đối số vị trí đầu tiên là link/ID Drive và không phải tên lệnh; các cờ
  global đứng trước (`--config`, `--endpoint`, `--token-file`, `--color`,
  `--timeout`) được bỏ qua cùng giá trị của chúng. Lệnh lạ vẫn là lỗi.
- Tham chiếu job ở mọi lệnh nhận `JOB_ID`: ID đầy đủ, **tiền tố duy nhất**,
  `@last` (mới nhất) và `@running` (mới nhất chưa kết thúc). Tiền tố khớp nhiều
  job → exit 2 nêu các ID; không khớp → để daemon trả `not_found` (exit 4).
- `502drive wait ID` là dạng ngắn của `jobs wait` (cùng `--wait-timeout`).
- `502drive watch [ID]` in mỗi giây một dòng; với `--json` là một envelope JSON
  mỗi giây. Mặc định theo `@last`; dừng khi job kết thúc với cùng mã thoát như
  `wait` (0/7/8).
- Completion động (bash/zsh/fish, qua `ValidArgsFunction`) cho mọi lệnh có
  `JOB_ID`/`COLLECTION` trong `Use`: ID job/collection thật từ daemon cộng
  `@last`/`@running`. Completion im lặng khi thiếu config/daemon.
- `502drive doctor --fix` gọi `POST /v1/doctor/fix`: tạo thư mục config/data còn
  thiếu với 0700, `chmod 0600` cho `token.json`, `google-token.enc`, `master.key`,
  `config.json`; sau đó chạy lại check. Không chạm Drive/job.

## Mã thoát

Giữ nguyên hợp đồng hiện có: `copy --wait`/`wait`/`watch` trả 0 khi `succeeded`,
7 khi `partial`/`failed`/`blocked`, 8 khi `cancelled`, 6 khi hết thời gian chờ.
Batch H không đổi các mã này (bảng trong prompt khác bảng đã công bố và đã có
test).

## Cửa sổ lỗi đã kiểm

| Tình huống | Kỳ vọng | Test |
| --- | --- | --- |
| Link trần (không có `copy`) | tạo job như copy | `TestBareLinkMeansCopyAndShortJobReferences` |
| Tiền tố khớp 2 job | exit 2, không đoán | cùng test |
| `@last` | job mới nhất | cùng test |
| `watch --json` | mỗi dòng một envelope có `data.id` | `TestWatchPrintsOneJSONLinePerTick` |
| Completion job/collection | trả ID thật + alias, directive NoFileComp | `TestShellCompletionListsJobAndCollectionIDs` |
| `doctor --fix` | chmod 0600, check chuyển passed | `TestDoctorFixRestrictsSecretPermissions`, [app/doctor_test.go](../../internal/app/doctor_test.go) `TestDoctorFixCreatesDirectoriesAndRestrictsSecrets` |

## Chưa làm

- Không resolve tiền tố cho audit ID (chưa có yêu cầu).
- Completion dùng 50 job/collection mới nhất; ID cũ hơn vẫn gõ được nhưng không
  hiện trong gợi ý.

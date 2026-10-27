# F1 — Fast-list: gộp truy vấn theo nhiều thư mục cha

Thiết kế cho §F1 (phase-13). Gốc: S2 + S3 trong working tree.

## Vấn đề

Discovery hiện gọi `files.list` **một lần cho mỗi thư mục** (`ListChildren`),
pageSize 200, `fields` đầy đủ metadata hiển thị. Cây 1k thư mục nhỏ = 1k lệnh
list, mỗi lệnh mang theo owner/time/webViewLink không dùng để map.

## Thiết kế

### `drive.BatchLister` (interface tuỳ chọn)

```go
type Parent struct{ ID, ResourceKey string }
type BatchLister interface {
    ListChildrenBatch(ctx context.Context, parents []Parent, pageToken string) ([]File, string, error)
}
```

- `Provider` **không** đổi (không phá test double/consumer cũ). Engine type-assert
  `r.p.(drive.BatchLister)`; không có thì dùng đường cũ từng thư mục.
- Một lệnh: `q = ('A' in parents or 'B' in parents …) and trashed=false`,
  tối đa `MaxBatchParents = 40` cha/lệnh, `pageSize(1000)`, `orderBy=folder,name`.
- `fields` tối thiểu dùng chung cho cả list đơn:
  `id,name,mimeType,parents,size,md5Checksum,version,resourceKey,trashed,shortcutDetails(...)`
  (bỏ createdTime/modifiedTime/owners/webViewLink/appProperties/capabilities/driveId).
- Resource key: header `X-Goog-Drive-Resource-Keys` ghép `id/key` cho các cha có
  key (Drive hỗ trợ nhiều cặp, phân tách bằng dấu phẩy).

### Engine

`discover`/`discoverDelta` gọi chung `scanTree(ctx, commit)`:

1. **Partial trước**: folder có `list_state='partial'` (page token dở dang) được
   list tiếp từng cái, vì token trang gắn với một query.
2. **Batch**: lấy tối đa 40 folder `list_state='none'` (`NextToListBatch`), query
   gộp và **drain hết trang** (query không có checkpoint riêng), gom children
   theo parent, rồi commit từng folder.
3. **Trần bộ nhớ**: nếu batch vượt `maxBatchFiles = 20000` file, bỏ kết quả và
   list từng folder (đường cũ). Vì vậy không bao giờ nạp cả cây vào RAM; tối đa
   một batch nhỏ.
4. Lỗi batch (quyền/400/…): fallback list từng folder để khoanh đúng folder lỗi;
   throttle/5xx retry như S3, không đốt attempt.
5. Commit dùng `CommitPage`/`CommitDeltaPage` y như cũ → checkpoint và
   idempotency không đổi.

**Thứ tự cha→con**: worker chỉ `ClaimItem` khi parent `state='done'`; batch chỉ
đổi cách *đọc*, không đổi cách *tạo*. Folder con vẫn được tạo sau folder cha.

### Vì sao drain cả batch?

Page token của Drive là token của **query**, không của từng folder; lưu nó vào
một folder sẽ sai. Drain trong một lần giữ code đơn giản và crash-safe: chưa
commit folder nào thì chưa đổi `list_state`; crash giữa batch chỉ list lại batch
đó (INSERT OR IGNORE/idempotent). Với cây nhiều folder nhỏ, mỗi batch chỉ 1
trang nên đây cũng là đường nhanh nhất.

### Đo lường

- `drivetest.Fake` đếm `Calls["list"]` và `Calls["list_batch"]`; có
  `PageSize` chỉnh được.
- Test nhỏ: 42 folder anh em → trước 43 lệnh list, sau ≤3 lệnh batch (≥14×).
- Test lớn (chạy tay, `-run TestFastListTenThousand`): cây 10k file/1k folder,
  so số lệnh + thời gian discovery trước/sau, RSS đỉnh; ghi vào
  `docs/verification.md`.

## Cửa sổ lỗi

| Cửa sổ | Ứng xử |
| --- | --- |
| Crash giữa batch trước commit | Không folder nào đổi `list_state`; chạy lại list lại batch, insert idempotent |
| Crash sau commit một phần batch | Folder đã commit giữ `done`; phần còn lại list lại |
| Batch lỗi quyền/một folder mất | Fallback từng folder → chỉ folder lỗi bị `FailListing` |
| Batch quá lớn | Fallback từng folder, giới hạn RAM |
| Throttle giữa batch | Retry cùng query với backoff + `Retry-After`, không tiêu attempt |
| Pause/cancel ngay đầu một trang | Trả lỗi huỷ, **không** commit trang rỗng (nếu commit sẽ đánh dấu folder đã list xong và mất phần còn lại) |
| Không thay đổi gì | Delta vẫn list mỗi folder một lần trong batch; không snapshot thừa, không copy, không ghi đích |
| Provider không hỗ trợ batch | Đường cũ nguyên vẹn |

## Test

- `drive/client_test.go`: query chứa đủ parents, pageSize/fields tối thiểu,
  header resource key ghép đúng, trang kế.
- `drivetest` + engine: 42 folder → số lệnh batch; mapping/đích giống hệt
  đường từng folder; thứ tự tạo cha trước con.
- Engine: sync không thay đổi → 0 lệnh ghi, số lệnh list = số batch của cây.
- Đo 10k/1k (tay) và ghi số.

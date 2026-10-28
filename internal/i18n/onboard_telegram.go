package i18n

// The Telegram step of `502drive onboard --telegram`: create a bot with
// @BotFather, paste its token, and let the bot see the owner's numeric id by
// having the owner send /start. Vietnamese and English.
const (
	OnbTgIntro     Key = "onb_tg_intro"
	OnbTgStep1     Key = "onb_tg_step1"
	OnbTgToken     Key = "onb_tg_token"
	OnbTgTokenBad  Key = "onb_tg_token_bad"
	OnbTgTokenFail Key = "onb_tg_token_fail"
	OnbTgBot       Key = "onb_tg_bot"
	OnbTgWaiting   Key = "onb_tg_waiting"
	OnbTgGot       Key = "onb_tg_got"
	OnbTgManual    Key = "onb_tg_manual"
	OnbTgIDBad     Key = "onb_tg_id_bad"
	OnbTgSaved     Key = "onb_tg_saved"
	OnbTgRestart   Key = "onb_tg_restart"
	OnbTgStart     Key = "onb_tg_start"
	OnbTgCancelled Key = "onb_tg_cancelled"
	OnbTgAlready   Key = "onb_tg_already"
	OnbTgTip       Key = "onb_tg_tip"
	OnbTgNeedsTTY  Key = "onb_tg_needs_tty"
)

func init() {
	vi := map[Key]string{
		OnbTgIntro:     "Telegram (tùy chọn): điều khiển 502Drive từ điện thoại. Cần 2 thứ: token của bot và ID số của bạn.",
		OnbTgStep1:     "1/3 Mở https://t.me/BotFather, gửi /newbot, đặt tên và username (phải kết thúc bằng \"bot\"). BotFather sẽ trả về một token dạng 123456:ABC…",
		OnbTgToken:     "Dán token bot (không hiện trên màn hình)",
		OnbTgTokenBad:  "Chuỗi này không giống token bot (dạng 123456:ABC…). Thử lại.",
		OnbTgTokenFail: "Telegram không nhận token này. Kiểm tra đã sao chép đủ chưa rồi thử lại.",
		OnbTgBot:       "Bot của bạn: @%s",
		OnbTgWaiting:   "2/3 Mở https://t.me/%s trong Telegram, bấm Start (hoặc gửi /start). Đang chờ tin nhắn của bạn…",
		OnbTgGot:       "Nhận được tin nhắn từ %s (ID %d). Đúng là bạn? [Y/n]",
		OnbTgManual:    "Không thấy tin nhắn nào. Dán ID số của bạn (lấy từ bot như @userinfobot), hoặc Enter để bỏ qua",
		OnbTgIDBad:     "ID phải là một số dương. Thử lại.",
		OnbTgSaved:     "3/3 Đã lưu bot Telegram. Token nằm trong thư mục cấu hình riêng tư, không in ra.",
		OnbTgRestart:   "Dịch vụ đang chạy: chạy `502drive stop` rồi `502drive up` để bot bắt đầu hoạt động.",
		OnbTgStart:     "Bot sẽ bắt đầu hoạt động khi dịch vụ khởi động (502drive up).",
		OnbTgCancelled: "Đã bỏ qua Telegram. Chạy lại `502drive onboard --telegram` khi cần.",
		OnbTgAlready:   "Telegram đã được thiết lập (token và ID chủ sở hữu). Không cần làm gì thêm.",
		OnbTgTip:       "Muốn điều khiển bằng Telegram? Chạy: 502drive onboard --telegram",
		OnbTgNeedsTTY:  "Bước Telegram cần một terminal để hỏi bạn. Với script, dùng 502drive setup --telegram-token-file FILE --telegram-owner-id ID.",
	}
	en := map[Key]string{
		OnbTgIntro:     "Telegram (optional): control 502Drive from your phone. You need two things: the bot token and your numeric id.",
		OnbTgStep1:     "1/3 Open https://t.me/BotFather, send /newbot, pick a name and a username (it must end in \"bot\"). BotFather replies with a token like 123456:ABC…",
		OnbTgToken:     "Paste the bot token (hidden)",
		OnbTgTokenBad:  "That does not look like a bot token (123456:ABC…). Try again.",
		OnbTgTokenFail: "Telegram did not accept this token. Check you copied all of it and try again.",
		OnbTgBot:       "Your bot: @%s",
		OnbTgWaiting:   "2/3 Open https://t.me/%s in Telegram and press Start (or send /start). Waiting for your message…",
		OnbTgGot:       "Got a message from %s (id %d). Is that you? [Y/n]",
		OnbTgManual:    "No message arrived. Paste your numeric id (from a bot such as @userinfobot), or Enter to skip",
		OnbTgIDBad:     "The id must be a positive number. Try again.",
		OnbTgSaved:     "3/3 The Telegram bot is saved. The token is in the private configuration folder and is not printed.",
		OnbTgRestart:   "The service is running: run `502drive stop` then `502drive up` so the bot starts.",
		OnbTgStart:     "The bot starts when the service starts (502drive up).",
		OnbTgCancelled: "Telegram skipped. Run `502drive onboard --telegram` when you want it.",
		OnbTgAlready:   "Telegram is already set up (token and owner id). Nothing to do.",
		OnbTgTip:       "Want to control it from Telegram? Run: 502drive onboard --telegram",
		OnbTgNeedsTTY:  "The Telegram step needs a terminal to ask you. For scripts use 502drive setup --telegram-token-file FILE --telegram-owner-id ID.",
	}
	for k, v := range vi {
		VI[k] = v
	}
	for k, v := range en {
		EN[k] = v
	}
}

package tray

import (
	"context"
	"github.com/godbus/dbus/v5"
	"time"
)

// notifyDesktop uses fixed action IDs; all remote names remain plain data.
func notifyDesktop(ctx context.Context, n Notice, open func()) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	if err = conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.Notifications")); err != nil {
		conn.Close()
		return err
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	actions := []string{"open", n.ActionLabel}
	var id uint32
	err = conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").CallWithContext(callCtx, "org.freedesktop.Notifications.Notify", 0, "502Drive", uint32(0), "", escapeMarkup(plain(n.Title)), escapeMarkup(plain(n.Body)), actions, map[string]dbus.Variant{}, int32(60000)).Store(&id)
	if err != nil {
		conn.RemoveSignal(signals)
		conn.Close()
		return err
	}
	go func() {
		defer conn.Close()
		defer conn.RemoveSignal(signals)
		timer := time.NewTimer(61 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if signal == nil || len(signal.Body) < 1 {
					continue
				}
				got, ok := signal.Body[0].(uint32)
				if !ok || got != id {
					continue
				}
				if signal.Name == "org.freedesktop.Notifications.ActionInvoked" && len(signal.Body) > 1 {
					if action, ok := signal.Body[1].(string); ok && action == "open" {
						open()
						return
					}
				}
				if signal.Name == "org.freedesktop.Notifications.NotificationClosed" {
					return
				}
			}
		}
	}()
	return nil
}

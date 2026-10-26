//go:build linux || windows

package tray

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
)

// runTray owns the systray event loop and the agent.
func runTray(ctx context.Context, o Options) error {
	// Headless Linux has no session bus: fail with a clear message instead of
	// blocking on a tray that can never appear.
	if o.Host.GOOS == "linux" && o.Host.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return errors.New("no desktop session for the tray (DBUS_SESSION_BUS_ADDRESS is not set); use `502drive menu` instead")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	a := newAgent(o)
	p := &systrayPresenter{items: map[string]*systray.MenuItem{}, now: o.now, motion: o.Motion}
	a.p = p

	done := make(chan struct{})
	go func() {
		<-runCtx.Done()
		systray.Quit()
	}()
	systray.Run(func() {
		p.build(a)
		systray.SetTitle("502Drive")
		systray.SetTooltip("502Drive")
		_ = p.setIcon(StateIdle, 0)
		go a.pollLoop(runCtx)
		go a.iconLoop(runCtx)
	}, func() {
		close(done)
	})
	<-done
	return nil
}

// systrayPresenter maps a View onto systray menu items. Items are created
// once; later refreshes only change titles and flags, so click handlers are
// never re-registered.
type systrayPresenter struct {
	mu     sync.Mutex
	items  map[string]*systray.MenuItem
	agent  *agent
	now    func() time.Time
	motion bool
}

func (p *systrayPresenter) build(a *agent) {
	p.agent = a
	skeleton := BuildView(Snapshot{}, false, "")
	for _, it := range skeleton.Items {
		if it.Separator {
			systray.AddSeparator()
			continue
		}
		if len(it.Children) > 0 {
			parent := systray.AddMenuItem(it.Title, "")
			p.items[it.ID] = parent
			for _, c := range it.Children {
				if c.Separator {
					parent.AddSeparator()
					continue
				}
				child := parent.AddSubMenuItem(c.Title, "")
				p.items[c.ID] = child
				p.watch(c.ID, child)
			}
			continue
		}
		item := systray.AddMenuItem(it.Title, "")
		p.items[it.ID] = item
		p.watch(it.ID, item)
	}
}

// watch dispatches one fixed action id; info rows have no click handler.
func (p *systrayPresenter) watch(id string, item *systray.MenuItem) {
	if id == "status" || strings.HasPrefix(id, jobSlotID+":") {
		return
	}
	go func() {
		for range item.ClickedCh {
			p.agent.action(context.Background(), id)
		}
	}()
}

func (p *systrayPresenter) show(v View) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, it := range v.Items {
		if it.Separator {
			continue
		}
		p.apply(it)
		for _, c := range it.Children {
			if c.Separator {
				continue
			}
			p.apply(c)
		}
	}
	systray.SetTooltip(v.Title)
	return p.setIconLocked(v.State, 0)
}

func (p *systrayPresenter) apply(it Item) {
	item, ok := p.items[it.ID]
	if !ok {
		return
	}
	if it.Hidden {
		item.Hide()
		return
	}
	item.Show()
	item.SetTitle(it.Title)
	if it.Enabled {
		item.Enable()
	} else {
		item.Disable()
	}
}

func (p *systrayPresenter) setIcon(st State, frame int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setIconLocked(st, frame)
}

func (p *systrayPresenter) setIconLocked(st State, frame int) error {
	data, err := PNG(st, frame%busyFrames, iconSize)
	if err != nil {
		return err
	}
	systray.SetIcon(data)
	return nil
}

func (p *systrayPresenter) quit() { systray.Quit() }

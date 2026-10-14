package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/elianiva/meiro/m3"
)

// logsCard is where the user finds the log file, which says what went wrong
// when something did.
func (a *app) logsCard(c *ui.Context) {
	a.settingsCard(c, "Logs", m3.IconFolder, func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			body := "Meiro keeps a log of what it does and what goes wrong, with no passwords or cookies in it."
			if a.logsDir == "" {
				body = "Meiro could not open its log file, so nothing is being logged."
			}
			settingLabel(c, "Log files", body).Grow(1)
			if m3.Button(c, m3.ButtonSpec{Label: "Open logs folder", Kind: m3.Tonal, Size: m3.Medium56, Disabled: a.logsDir == "", Key: "open-logs"}).Clicked() {
				a.openLogs()
			}
		})
	})
}

// historyCard lets the user remove locally stored plays used for stats.
func (a *app) historyCard(c *ui.Context) {
	a.settingsCard(c, "Listening history", m3.IconHistory, func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			settingLabel(c, "Plays on this device", "This history powers your listening stats.").Grow(1)
			if m3.Button(c, m3.ButtonSpec{Label: "Clear history", Kind: m3.Outlined, Size: m3.Medium56, Key: "clear-history"}).Clicked() {
				a.clearHistory()
			}
		})
	})
}

func (a *app) openLogs() {
	dir := a.logsDir
	if dir == "" {
		return
	}
	spawn(a, func() error { return a.openPath(dir) }, func(err error) {
		if err != nil {
			a.notice = "Could not open the logs folder: " + err.Error()
		}
	})
}

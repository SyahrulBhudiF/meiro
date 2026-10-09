package youtube

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestCookieAuthRedactsFormattingAndStructuredLogs(t *testing.T) {
	const cookie = "SAPISID=top-secret; SID=session-secret"
	auth, err := NewCookieAuth(cookie, CookieOptions{})
	if err != nil {
		t.Fatal(err)
	}

	formatted := fmt.Sprintf("%v\n%+v\n%#v\n%v\n%+v\n%#v", auth, auth, auth, *auth, *auth, *auth)
	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("request", "pointer_auth", auth, "value_auth", *auth)

	for name, output := range map[string]string{
		"formatted": formatted,
		"slog":      logged.String(),
	} {
		if strings.Contains(output, cookie) || strings.Contains(output, "top-secret") || strings.Contains(output, "session-secret") {
			t.Errorf("%s output exposed cookie contents: %q", name, output)
		}
	}
}

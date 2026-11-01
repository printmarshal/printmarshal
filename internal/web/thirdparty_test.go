package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A self-hosted console contacts nothing but its own server when a person opens
// it, so its page and stylesheet name no other host.
func TestPageNamesNoThirdPartyHost(t *testing.T) {
	url := regexp.MustCompile(`https?://[^\s"'<>)]+`)
	for _, file := range []string{"../../web/index.html", "../../web/src/styles.css"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range url.FindAllString(string(b), -1) {
			if !strings.HasPrefix(u, "http://www.w3.org/") { // an SVG namespace, never fetched
				t.Errorf("%s names %s", file, u)
			}
		}
	}
}

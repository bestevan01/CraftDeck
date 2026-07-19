package update

import (
	"fmt"
	"os"
	"os/exec"
)

// sourcesListPath is the one file install.sh writes at install time (see
// packaging/apt-repo/install.sh) -- a single `deb ...` line, nothing else,
// so switching channels can safely overwrite it wholesale rather than
// find-and-replacing a managed block within a larger file (unlike
// hardware.ApplyConfig's config.txt, which has to coexist with an
// operator's own unrelated content).
const sourcesListPath = "/etc/apt/sources.list.d/craftdeck.list"

const (
	craftdeckKeyringPath = "/usr/share/keyrings/craftdeck.gpg"
	craftdeckRepoURL     = "https://apt.apple-farm.online"
)

// ApplySourcesList rewrites craftdeck.list to point at the given channel's
// apt component and refreshes apt's index immediately, so a channel switch
// takes effect for the very next version check/update without the
// operator needing to run `apt update` themselves. Synchronous: unlike
// hardware.ApplyConfig (needs a reboot) or handleUpdateCraftdeck (kills its
// own process), `apt-get update` finishes in a second or two and doesn't
// touch craftdeckd's own process, so there's nothing to detach here.
func ApplySourcesList(channel Channel) error {
	line := fmt.Sprintf("deb [arch=arm64 signed-by=%s] %s trixie %s\n",
		craftdeckKeyringPath, craftdeckRepoURL, channel.AptComponent())
	if err := os.WriteFile(sourcesListPath, []byte(line), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", sourcesListPath, err)
	}
	if err := exec.Command("apt-get", "update").Run(); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	return nil
}

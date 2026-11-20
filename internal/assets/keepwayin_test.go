package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// With a VPN client up, replies from the local address and traffic to the
// local network keep the way out they had: table 5400, through the local
// gateway; a default route into the tunnel is passed over.
func TestKeepWayIn(t *testing.T) {
	for name, defaults := range map[string]string{
		"wireguard": "default via 192.168.0.1 dev eth0 proto dhcp src 192.168.0.105 metric 100",
		"openvpn":   "default via 10.135.0.1 dev tun0\ndefault via 192.168.0.1 dev eth0 proto dhcp src 192.168.0.105 metric 100",
	} {
		dir := t.TempDir()
		fake := `#!/bin/sh
echo "ip $*" >> "` + dir + `/ran"
case "$*" in
  "-4 route show default table main") printf '%s\n' "` + defaults + `" ;;
  "-4 -o addr show dev eth0") echo "2: eth0    inet 192.168.0.105/24 brd 192.168.0.255 scope global dynamic eth0" ;;
  "-4 route show table main dev eth0 scope link") echo "192.168.0.0/24 proto kernel scope link src 192.168.0.105" ;;
  "rule del pref 5400") exit 1 ;;
esac
exit 0
`
		if err := os.WriteFile(filepath.Join(dir, "ip"), []byte(fake), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "conf/yardo-keep-way-in")
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "ran"))
		ran := string(b)
		for _, want := range []string{"ip route flush table 5400", "ip route replace 192.168.0.0/24 dev eth0 src 192.168.0.105 table 5400",
			"ip rule add to 192.168.0.0/24 lookup 5400 pref 5400", "ip rule add from 192.168.0.105 lookup 5400 pref 5400",
			"ip route replace default via 192.168.0.1 dev eth0 table 5400"} {
			if !strings.Contains(ran, want) {
				t.Errorf("%s: didn't run %q; ran:\n%s", name, want, ran)
			}
		}
		if strings.Contains(ran, "tun0 table") {
			t.Errorf("%s: the tunnel's route taken:\n%s", name, ran)
		}
	}
}

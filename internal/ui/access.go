package ui

// what each access choice means, in a sentence.
var accessSay = map[string]map[string]string{
	"mcp": {
		"127.0.0.1": "Only programs on this machine can use it.",
		"vpn":       "Your laptop too, once it's on Tailscale, NetBird or WireGuard, with a token.",
		"public":    "Anyone with a token, over HTTPS at your domain name.",
		"0.0.0.0":   "Anyone who can reach this machine, with a token, over plain HTTP.",
	},
	"ports": {
		"all":   "Ports you publish with docker run -p are open to every network.",
		"vpn":   "Published ports only from this machine and your VPNs.",
		"local": "Published ports only from this machine.",
	},
	"api": {
		"off": "Use Docker from your laptop over SSH. Nothing else opens.",
		"lan": "Port 2376 with client certificates, from your network and VPNs.",
		"vpn": "Port 2376 with client certificates, from your VPNs only.",
	},
}

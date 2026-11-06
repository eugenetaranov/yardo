package main

import (
	"fmt"
	"testing"
)

func TestParseContainers(t *testing.T) {
	// By image; a name of its own is kept, one docker made up isn't.
	out := "mcpjungle\tghcr.io/mcpjungle/mcpjungle:0.4.6\t127.0.0.1:8080->8080/tcp\n" +
		"myapp-db-1\tdocker.io/library/postgres:16\t0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp\n" +
		"clever_franklin\tredis\t\n" +
		"relaxed_brahmagupta\tbusybox@sha256:0123456789abcdef\t\n"
	got := fmt.Sprint(parseContainers(out))
	if want := "[{mcpjungle:0.4.6  [8080]} {postgres:16 myapp-db-1 [5432]} {redis  []} {busybox@0123456  []}]"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if cs := parseContainers(""); len(cs) != 0 {
		t.Errorf("none running: %v", cs)
	}
}

// The services that run or failed, once each, in the dashboard's order;
// stopped ones left out.
func TestParseServices(t *testing.T) {
	out := `fail2ban.service loaded failed failed Fail2Ban Service
ssh.service loaded active running OpenBSD Secure Shell server
docker.service loaded active running Docker Application Container Engine
nfs-server.service loaded inactive dead NFS server and services
wg-quick@wg0.service loaded active exited WireGuard via wg-quick(8) for wg0
cron.service loaded active running Regular background program processing daemon
`
	got := fmt.Sprint(parseServices(out))
	if want := "[{ssh true} {docker true} {wireguard true} {fail2ban false}]"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// What listens: named, its reach said, IPv4 and IPv6 once, the quiet ones
// (DHCP, the resolver) left out.
func TestParseListening(t *testing.T) {
	out := `udp   UNCONN 0      0            0.0.0.0:41641      0.0.0.0:*    users:(("tailscaled",pid=812,fd=17))
udp   UNCONN 0      0      10.0.0.5%eth0:68         0.0.0.0:*    users:(("systemd-network",pid=600,fd=22))
tcp   LISTEN 0      4096   127.0.0.53%lo:53         0.0.0.0:*    users:(("systemd-resolve",pid=500,fd=14))
tcp   LISTEN 0      128          0.0.0.0:22         0.0.0.0:*    users:(("sshd",pid=900,fd=3))
tcp   LISTEN 0      128             [::]:22            [::]:*    users:(("sshd",pid=900,fd=4))
tcp   LISTEN 0      64           0.0.0.0:2049       0.0.0.0:*
tcp   LISTEN 0      4096   100.101.1.2:8080        0.0.0.0:*    users:(("docker-proxy",pid=1200,fd=4))
tcp   LISTEN 0      4096       127.0.0.1:631        0.0.0.0:*    users:(("cupsd",pid=700,fd=7))
tcp   LISTEN 0      4096        10.0.0.5:9000       0.0.0.0:*    users:(("minio",pid=1300,fd=7))
`
	got := fmt.Sprint(parseListening(out))
	want := "[{41641 udp tailscale all} {22 tcp ssh all} {2049 tcp nfs all} {8080 tcp docker vpn} {631 tcp cupsd local} {9000 tcp minio 10.0.0.5}]"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

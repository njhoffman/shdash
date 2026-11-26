package collect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/njhoffman/shdash/internal/format"
)

type SystemData struct {
	Host     string    `json:"host"`
	OS       string    `json:"os"`
	Kernel   string    `json:"kernel"`
	CPU      string    `json:"cpu"`
	Threads  int       `json:"threads"`
	Boot     time.Time `json:"boot"`
	MemUsed  uint64    `json:"mem_used"`
	MemTotal uint64    `json:"mem_total"`
	Disks    []Disk    `json:"disks"`
	Battery  *Battery  `json:"battery,omitempty"`
	Packages int       `json:"packages"`
}

type Disk struct {
	Mount string `json:"mount"`
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

type Battery struct {
	Capacity float64 `json:"capacity"`
	Status   string  `json:"status"`
}

// System is fastfetch's view of the host.
func System() Collector {
	return Collector{
		Name: "system", Title: "System", Icon: "\U000F0379", Priority: 1,
		TTL: 2 * time.Minute, Timeout: 10 * time.Second, Inline: true, Schema: 1,
		Collect: func(ctx context.Context, env *Env) (any, error) {
			out, err := env.Run(ctx, "fastfetch", "--format", "json", "--structure",
				"OS:Host:Kernel:Uptime:CPU:Memory:Disk:Battery:Packages")
			if err != nil {
				return nil, err
			}
			return parseFastfetch(out, env.Now(), env.Home)
		},
		Lines: linesOf(systemLines),
	}
}

func parseFastfetch(out []byte, now time.Time, home string) (*SystemData, error) {
	var mods []struct {
		Type   string          `json:"type"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(out, &mods); err != nil {
		return nil, fmt.Errorf("fastfetch: %w", err)
	}
	d := &SystemData{}
	for _, m := range mods {
		if m.Error != "" || len(m.Result) == 0 || string(m.Result) == "null" {
			continue
		}
		// Field names match fastfetch's keys case-insensitively.
		switch m.Type {
		case "OS":
			var r struct{ PrettyName, Name string }
			if json.Unmarshal(m.Result, &r) == nil {
				d.OS = first(r.PrettyName, r.Name)
			}
		case "Host":
			var r struct{ Family, Name string }
			if json.Unmarshal(m.Result, &r) == nil {
				d.Host = first(r.Family, r.Name)
			}
		case "Kernel":
			var r struct{ Release string }
			if json.Unmarshal(m.Result, &r) == nil {
				d.Kernel = r.Release
			}
		case "Uptime":
			var r struct{ Uptime int64 }
			if json.Unmarshal(m.Result, &r) == nil && r.Uptime > 0 {
				d.Boot = now.Add(-time.Duration(r.Uptime) * time.Millisecond)
			}
		case "CPU":
			var r struct {
				CPU   string
				Cores struct{ Logical int }
			}
			if json.Unmarshal(m.Result, &r) == nil {
				d.CPU, d.Threads = cpuName(r.CPU), r.Cores.Logical
			}
		case "Memory":
			var r struct{ Total, Used uint64 }
			if json.Unmarshal(m.Result, &r) == nil {
				d.MemTotal, d.MemUsed = r.Total, r.Used
			}
		case "Disk":
			var r []struct {
				Mountpoint string
				Bytes      struct{ Total, Used uint64 }
			}
			if json.Unmarshal(m.Result, &r) == nil {
				for _, x := range r {
					if x.Mountpoint == "/" || x.Mountpoint == "/home" || (home != "" && x.Mountpoint == home) {
						d.Disks = append(d.Disks, Disk{x.Mountpoint, x.Bytes.Used, x.Bytes.Total})
					}
				}
			}
		case "Battery":
			var r []Battery
			if json.Unmarshal(m.Result, &r) == nil && len(r) > 0 {
				d.Battery = &r[0]
			}
		case "Packages":
			var r struct{ All int }
			if json.Unmarshal(m.Result, &r) == nil {
				d.Packages = r.All
			}
		}
	}
	if d.OS == "" && d.Kernel == "" && d.Host == "" {
		return nil, errors.New("fastfetch: no modules in its output")
	}
	return d, nil
}

func first(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// cpuName drops the trademark marks and the "CPU @ 2.90GHz" tail.
func cpuName(s string) string {
	for _, junk := range []string{"(R)", "(TM)", "(tm)"} {
		s = strings.ReplaceAll(s, junk, "")
	}
	if i := strings.Index(s, " @ "); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), " CPU")
	return strings.Join(strings.Fields(s), " ")
}

func systemLines(d *SystemData, env *Env) []string {
	var l []string
	if d.Host != "" {
		l = append(l, format.KV("host", d.Host))
	}
	if os := format.Join(d.OS, d.Kernel); os != "" {
		l = append(l, format.KV("os", os))
	}
	if !d.Boot.IsZero() {
		l = append(l, format.KV("up", format.Age(env.Now().Sub(d.Boot))))
	}
	if d.CPU != "" {
		threads := ""
		if d.Threads > 0 {
			threads = format.Count(d.Threads, "thread", "threads", true)
		}
		l = append(l, format.KV("cpu", format.Join(d.CPU, threads)))
	}
	if d.MemTotal > 0 {
		l = append(l, format.KV("mem", usage(d.MemUsed, d.MemTotal)))
	}
	for _, x := range d.Disks {
		l = append(l, format.KV("disk", x.Mount+"  "+usage(x.Used, x.Total)))
	}
	if b := d.Battery; b != nil {
		l = append(l, format.KV("battery", format.Join(fmt.Sprintf("%.0f%%", b.Capacity), b.Status)))
	}
	if d.Packages > 0 {
		l = append(l, format.KV("pkgs", fmt.Sprint(d.Packages)))
	}
	return l
}

func usage(used, total uint64) string {
	pct := format.Pct(used, total)
	s := fmt.Sprintf("%s/%s GiB %d%%", format.GiB(used), format.GiB(total), pct)
	if pct >= 90 {
		return format.Strong.Render(s)
	}
	return s
}

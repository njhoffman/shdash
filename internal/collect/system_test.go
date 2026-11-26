package collect

import (
	"context"
	"strings"
	"testing"
	"time"
)

const ffArgs = "fastfetch --format json --structure OS:Host:Kernel:Uptime:CPU:Memory:Disk:Battery:Packages"

func TestSystemFromFastfetch(t *testing.T) {
	env := fakeEnv(map[string]string{ffArgs: fixture(t, "fastfetch.json")}, nil, nil)
	l := lines(t, System(), env)
	assertHas(t, l, "Yoga 9 14ITL5", "Ubuntu 25.10", "6.17.0-41-generic",
		"Intel Core i7-1195G7", "8 threads", "GiB", "disk    /  ", "99%", "AC Connected")
	// Only / and the home mount are shown, not /boot/efi.
	assertLacks(t, l, "/boot/efi", "(R)", "(TM)")
}

func TestSystemUptimeAgesFromCollection(t *testing.T) {
	ff := `[{"type":"OS","result":{"prettyName":"Test OS"}},{"type":"Uptime","result":{"uptime":3600000}}]`
	env := fakeEnv(map[string]string{ffArgs: ff}, nil, nil)
	data, err := System().Collect(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	// Two hours later the cached boot time gives 3h, not the 1h fastfetch saw.
	later := fakeEnv(nil, nil, nil)
	later.Now = func() time.Time { return testNow.Add(2 * time.Hour) }
	assertHas(t, render(t, System(), later, data), "up      3h")
}

func TestSystemSkipsFailedModules(t *testing.T) {
	ff := `[{"type":"OS","result":{"prettyName":"Test OS"}},
	        {"type":"Battery","error":"No batteries found"},
	        {"type":"Memory","result":null}]`
	l := lines(t, System(), fakeEnv(map[string]string{ffArgs: ff}, nil, nil))
	assertHas(t, l, "Test OS")
	assertLacks(t, l, "battery", "mem")
}

func TestSystemErrors(t *testing.T) {
	for name, out := range map[string]string{
		"no fastfetch": "",
		"bad json":     "not json",
		"no modules":   `[{"type":"Battery","error":"none"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			outputs := map[string]string{}
			if out != "" {
				outputs[ffArgs] = out
			}
			if _, err := System().Collect(context.Background(), fakeEnv(outputs, nil, nil)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestCPUName(t *testing.T) {
	for in, want := range map[string]string{
		"11th Gen Intel(R) Core(TM) i7-1195G7 @ 2.90GHz": "11th Gen Intel Core i7-1195G7",
		"AMD Ryzen 7 5800X 8-Core Processor":             "AMD Ryzen 7 5800X 8-Core Processor",
		"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz":      "Intel Xeon CPU E5-2680 v4",
		"Intel(R) Core(TM) Ultra 9 285H CPU":             "Intel Core Ultra 9 285H",
	} {
		if got := cpuName(in); got != want {
			t.Errorf("cpuName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsageMarksFullDisks(t *testing.T) {
	if s := usage(95, 100); !strings.Contains(s, "95%") {
		t.Errorf("usage = %q", s)
	}
}

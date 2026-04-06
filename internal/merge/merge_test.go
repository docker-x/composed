package merge

import (
	"testing"

	"github.com/docker-x/composed/internal/compose"
)

func TestMerge_SingleFragment(t *testing.T) {
	f := compose.NewFile()
	f.Services["web"] = compose.NewService("nginx:latest")
	f.Volumes["data"] = &compose.Volume{}

	result := Merge("test", f)

	if result.Project != "test" {
		t.Errorf("Project = %q, want %q", result.Project, "test")
	}
	if len(result.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(result.Services))
	}
	if result.Services["web"].Image != "nginx:latest" {
		t.Errorf("Image = %q", result.Services["web"].Image)
	}
	if len(result.Volumes) != 1 {
		t.Errorf("Volumes count = %d, want 1", len(result.Volumes))
	}
}

func TestMerge_TwoFragments_DifferentServices(t *testing.T) {
	f1 := compose.NewFile()
	f1.Services["web"] = compose.NewService("nginx:latest")

	f2 := compose.NewFile()
	f2.Services["db"] = compose.NewService("postgres:15")

	result := Merge("test", f1, f2)

	if len(result.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(result.Services))
	}
	if result.Services["web"].Image != "nginx:latest" {
		t.Error("missing web service")
	}
	if result.Services["db"].Image != "postgres:15" {
		t.Error("missing db service")
	}
}

func TestMerge_ConflictingServices_Merged(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:v1")
	svc1.Environment["FOO"] = "bar"
	svc1.Ports = []string{"8080:8080"}
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("app:v2")
	svc2.Environment["BAZ"] = "qux"
	svc2.Ports = []string{"9090:9090"}
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)

	app := result.Services["app"]
	if app.Image != "app:v2" {
		t.Errorf("Image = %q, want %q (later wins)", app.Image, "app:v2")
	}
	if app.Environment["FOO"] != "bar" {
		t.Error("FOO should be preserved from first fragment")
	}
	if app.Environment["BAZ"] != "qux" {
		t.Error("BAZ should be added from second fragment")
	}
	if len(app.Ports) != 2 {
		t.Errorf("Ports = %v, want 2 unique ports", app.Ports)
	}
}

func TestMerge_EnvironmentOverride(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:latest")
	svc1.Environment["KEY"] = "old"
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("")
	svc2.Environment["KEY"] = "new"
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)
	if result.Services["app"].Environment["KEY"] != "new" {
		t.Errorf("KEY = %q, want %q (later wins)", result.Services["app"].Environment["KEY"], "new")
	}
}

func TestMerge_PortDedup(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:latest")
	svc1.Ports = []string{"8080:8080", "9090:9090"}
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("")
	svc2.Ports = []string{"8080:8080", "3000:3000"}
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)
	app := result.Services["app"]
	if len(app.Ports) != 3 {
		t.Errorf("Ports = %v, want 3 unique ports", app.Ports)
	}
}

func TestMerge_VolumeUnion(t *testing.T) {
	f1 := compose.NewFile()
	f1.Volumes["data"] = &compose.Volume{}
	f1.Volumes["shared"] = &compose.Volume{Driver: "nfs"}

	f2 := compose.NewFile()
	f2.Volumes["logs"] = &compose.Volume{}
	f2.Volumes["shared"] = &compose.Volume{Driver: "local"} // should not override

	result := Merge("test", f1, f2)

	if len(result.Volumes) != 3 {
		t.Errorf("Volumes count = %d, want 3", len(result.Volumes))
	}
	if result.Volumes["shared"].Driver != "nfs" {
		t.Errorf("shared driver = %q, want %q (first wins)", result.Volumes["shared"].Driver, "nfs")
	}
}

func TestMerge_NetworkUnion(t *testing.T) {
	f1 := compose.NewFile()
	f1.Networks["frontend"] = &compose.Network{}

	f2 := compose.NewFile()
	f2.Networks["backend"] = &compose.Network{}

	result := Merge("test", f1, f2)
	if len(result.Networks) != 2 {
		t.Errorf("Networks count = %d, want 2", len(result.Networks))
	}
}

func TestMerge_ConfigUnion(t *testing.T) {
	f1 := compose.NewFile()
	f1.Configs["app-config"] = &compose.Config{Content: "v1"}

	f2 := compose.NewFile()
	f2.Configs["db-config"] = &compose.Config{Content: "v2"}
	f2.Configs["app-config"] = &compose.Config{Content: "v2-override"} // should not override

	result := Merge("test", f1, f2)
	if len(result.Configs) != 2 {
		t.Errorf("Configs count = %d, want 2", len(result.Configs))
	}
	if result.Configs["app-config"].Content != "v1" {
		t.Errorf("app-config = %q, want %q (first wins)", result.Configs["app-config"].Content, "v1")
	}
}

func TestMerge_NilFragments(t *testing.T) {
	f1 := compose.NewFile()
	f1.Services["web"] = compose.NewService("nginx:latest")

	result := Merge("test", nil, f1, nil)
	if len(result.Services) != 1 {
		t.Errorf("Services count = %d, want 1", len(result.Services))
	}
}

func TestMerge_DependsOnMerge(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:latest")
	svc1.DependsOn["db"] = compose.DependsOnCondition{Condition: "service_healthy"}
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("")
	svc2.DependsOn["cache"] = compose.DependsOnCondition{Condition: "service_started"}
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)
	app := result.Services["app"]
	if len(app.DependsOn) != 2 {
		t.Errorf("DependsOn count = %d, want 2", len(app.DependsOn))
	}
}

func TestMerge_LabelsMerge(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:latest")
	svc1.Labels["team"] = "backend"
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("")
	svc2.Labels["env"] = "prod"
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)
	app := result.Services["app"]
	if app.Labels["team"] != "backend" {
		t.Error("missing team label")
	}
	if app.Labels["env"] != "prod" {
		t.Error("missing env label")
	}
}

func TestMerge_ScalarOverrides(t *testing.T) {
	f1 := compose.NewFile()
	svc1 := compose.NewService("app:v1")
	svc1.Restart = "always"
	svc1.Entrypoint = []string{"/old"}
	svc1.Command = []string{"old-cmd"}
	svc1.Healthcheck = &compose.Healthcheck{Test: []string{"CMD", "old"}}
	f1.Services["app"] = svc1

	f2 := compose.NewFile()
	svc2 := compose.NewService("app:v2")
	svc2.Restart = "unless-stopped"
	svc2.Entrypoint = []string{"/new"}
	svc2.Command = []string{"new-cmd"}
	svc2.Healthcheck = &compose.Healthcheck{Test: []string{"CMD", "new"}}
	replicas := 3
	svc2.Deploy = &compose.Deploy{Replicas: &replicas}
	f2.Services["app"] = svc2

	result := Merge("test", f1, f2)
	app := result.Services["app"]

	if app.Image != "app:v2" {
		t.Errorf("Image = %q", app.Image)
	}
	if app.Restart != "unless-stopped" {
		t.Errorf("Restart = %q", app.Restart)
	}
	if app.Entrypoint[0] != "/new" {
		t.Errorf("Entrypoint = %v", app.Entrypoint)
	}
	if app.Command[0] != "new-cmd" {
		t.Errorf("Command = %v", app.Command)
	}
	if app.Healthcheck.Test[1] != "new" {
		t.Errorf("Healthcheck.Test = %v", app.Healthcheck.Test)
	}
	if app.Deploy == nil || *app.Deploy.Replicas != 3 {
		t.Error("Deploy should be overridden")
	}
}

func TestAppendUnique(t *testing.T) {
	tests := []struct {
		name  string
		dst   []string
		items []string
		want  int
	}{
		{"no dups", []string{"a", "b"}, []string{"c", "d"}, 4},
		{"with dups", []string{"a", "b"}, []string{"b", "c"}, 3},
		{"all dups", []string{"a", "b"}, []string{"a", "b"}, 2},
		{"empty dst", nil, []string{"a", "b"}, 2},
		{"empty items", []string{"a"}, nil, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendUnique(tt.dst, tt.items...)
			if len(result) != tt.want {
				t.Errorf("appendUnique() len = %d, want %d", len(result), tt.want)
			}
		})
	}
}

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// devpods gives compose DEVPOD_SUFFIX (default `localhost`), and each Traefik
// router answers at two names: the `*.localhost` name of this machine and the
// name with the suffix, which another machine on the tailnet reaches
// (`*.magiklead.z13.internal` on the Z13). These tests render the devpod
// compose file with `docker compose config`, as devpods does, and read what
// compose gives Traefik and the containers.

// composeConfig is the rendered devpod/compose.yml, for the env below. The
// test skips when docker or its compose plugin is absent. devpods adds its
// postgres, redis and minio modules to the project at `devpods up`, and
// writes the secrets files; stubs take their place here. The stub
// .env.backend and .env.frontend hold the addresses of the devpods secrets
// files, so the render shows which value each container gets.
func composeConfig(t *testing.T, env ...string) composeProject {
	t.Helper()
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is not on PATH: the devpod compose file cannot render")
	}
	if err := exec.Command(docker, "compose", "version").Run(); err != nil {
		t.Skipf("docker has no compose plugin: %v", err)
	}
	dir := t.TempDir()
	write := func(name, text string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stubs := write("modules.yml", "services:\n"+
		"  postgres:\n    image: postgres\n"+
		"  redis:\n    image: redis\n"+
		"  minio-setup:\n    image: minio/mc\n")
	write(".env.backend", "FRONTEND_URL=https://mvp.magiklead.localhost\n")
	write(".env.frontend", "NEXT_PUBLIC_API_URL=http://api-mvp.magiklead.localhost\n")

	// devpods pins --project-directory to the repo root. The empty
	// --env-file keeps a gitignored root .env (the smoke worktree sets
	// MAGIKLEAD_PUBLIC_API_URL in it) out of the render.
	root := filepath.Join("..", "..", "..")
	file := filepath.Join(root, "devpod", "compose.yml")
	envFile := write("compose.env", "")
	cmd := exec.Command(docker, "compose", "--project-directory", root, "--env-file", envFile,
		"-f", file, "-f", stubs, "config", "--format", "json")
	cmd.Env = append(shellEnvWithoutPod(), append([]string{"SECRETS_DIR=" + dir}, env...)...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exit, ok := err.(*exec.ExitError); ok {
			stderr = string(exit.Stderr)
		}
		t.Fatalf("docker compose config: %v\n%s", err, stderr)
	}
	var project composeProject
	if err := json.Unmarshal(out, &project); err != nil {
		t.Fatalf("decode docker compose config: %v", err)
	}
	return project
}

// shellEnvWithoutPod is the env of the test with no variable that the
// compose file reads: a test run inside a devpod, or a shell with
// COMPOSE_FILE set, must not change the render.
func shellEnvWithoutPod() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "DEVPOD_") || strings.HasPrefix(name, "COMPOSE_") ||
			name == "MAGIKLEAD_PUBLIC_API_URL" || name == "SECRETS_DIR" {
			continue
		}
		env = append(env, kv)
	}
	return env
}

type composeProject struct {
	Services map[string]struct {
		Environment map[string]*string `json:"environment"`
		Labels      map[string]string  `json:"labels"`
	} `json:"services"`
}

var podEnv = []string{
	"DEVPOD_NAME=app",
	"DEVPOD_PROJECT=magiklead",
	"DEVPOD_SUFFIX=z13.internal",
}

func TestDevpodRoutersAnswerAtBothNames(t *testing.T) {
	project := composeConfig(t, podEnv...)

	want := map[string]string{
		"traefik.http.routers.api-magiklead-app.rule":      "Host(`api-app.magiklead.localhost`) || Host(`api-app.magiklead.z13.internal`)",
		"traefik.http.routers.frontend-magiklead-app.rule": "Host(`app.magiklead.localhost`) || Host(`app.magiklead.z13.internal`)",
	}
	rules := 0
	for name, service := range project.Services {
		for key, value := range service.Labels {
			if !strings.HasPrefix(key, "traefik.http.routers.") || !strings.HasSuffix(key, ".rule") {
				continue
			}
			rules++
			if want[key] != value {
				t.Errorf("service %s: %s = %q, want %q", name, key, value, want[key])
			}
		}
	}
	if rules != len(want) {
		t.Errorf("the compose file has %d router rules, want %d", rules, len(want))
	}
}

// With no suffix (devpods older than the suffix), the second name of each
// router falls back to the `.localhost` name, not to a host that ends in a dot.
func TestDevpodRoutersWithNoSuffix(t *testing.T) {
	project := composeConfig(t, "DEVPOD_NAME=app", "DEVPOD_PROJECT=magiklead")
	for name, service := range project.Services {
		for key, value := range service.Labels {
			if strings.HasPrefix(key, "traefik.http.routers.") && strings.HasSuffix(key, ".rule") &&
				strings.Contains(value, ".`)") {
				t.Errorf("service %s: %s = %q has a host that ends in a dot", name, key, value)
			}
		}
	}
}

// envValue is the value that compose gives a variable of a service, or ""
// when the service does not set it.
func envValue(t *testing.T, project composeProject, service, key string) string {
	t.Helper()
	s, ok := project.Services[service]
	if !ok {
		t.Fatalf("the compose file has no service %q", service)
	}
	if v := s.Environment[key]; v != nil {
		return *v
	}
	return ""
}

// The 4 address variables name the suffix name, so a browser on the other
// machine reaches them. With no suffix (devpods older than the suffix), they
// fall back to the `.localhost` name.
func TestDevpodAddressVariablesUseTheSuffix(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		host string
	}{
		{"suffix", podEnv, "magiklead.z13.internal"},
		{"no suffix", []string{"DEVPOD_NAME=app", "DEVPOD_PROJECT=magiklead"}, "magiklead.localhost"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			project := composeConfig(t, c.env...)
			want := []struct{ service, key, value string }{
				{"api", "APP_URL", "https://api-app." + c.host},
				{"api", "FRONTEND_URL", "https://app." + c.host},
				{"frontend", "NEXT_PUBLIC_API_URL", "https://api-app." + c.host},
				{"frontend", "NEXT_PUBLIC_APP_URL", "https://app." + c.host},
			}
			for _, w := range want {
				if got := envValue(t, project, w.service, w.key); got != w.value {
					t.Errorf("service %s: %s = %q, want %q", w.service, w.key, got, w.value)
				}
			}
		})
	}
}

// The smoke worktree sets MAGIKLEAD_PUBLIC_API_URL to its public tunnel
// origin. That value still wins over the suffix name.
func TestDevpodPublicAPIURLOverridesAppURL(t *testing.T) {
	project := composeConfig(t, append([]string{"MAGIKLEAD_PUBLIC_API_URL=https://smoke.example.com"}, podEnv...)...)
	if got := envValue(t, project, "api", "APP_URL"); got != "https://smoke.example.com" {
		t.Errorf("APP_URL = %q, want the MAGIKLEAD_PUBLIC_API_URL value", got)
	}
}

// The api allows the browser at both names of the frontend: FRONTEND_URL
// gives the suffix origin, CORS_EXTRA_ORIGINS the `.localhost` origin.
func TestDevpodAPIAllowsBothFrontendOrigins(t *testing.T) {
	project := composeConfig(t, podEnv...)
	opts := corsOptions(envValue(t, project, "api", "FRONTEND_URL"), envValue(t, project, "api", "CORS_EXTRA_ORIGINS"))
	for _, origin := range []string{"https://app.magiklead.z13.internal", "https://app.magiklead.localhost"} {
		if got := preflight(t, opts, origin); got != origin {
			t.Errorf("preflight from %s: Access-Control-Allow-Origin = %q, want %q", origin, got, origin)
		}
	}
}

// The Stripe, Unipile, Gmail and erasure handlers use FRONTEND_URL as the
// base of a redirect or a link, so it is one URL: the suffix name. The
// value of the compose file wins over the FRONTEND_URL of the devpods
// secrets file (.env.backend).
func TestDevpodFrontendURLIsOneBaseURL(t *testing.T) {
	project := composeConfig(t, podEnv...)
	got := envValue(t, project, "api", "FRONTEND_URL")
	if strings.Contains(got, ",") {
		t.Fatalf("FRONTEND_URL = %q is not one URL", got)
	}
	if want := "https://app.magiklead.z13.internal"; got != want {
		t.Errorf("FRONTEND_URL = %q, want %q (the compose value, not the secrets file)", got, want)
	}
}

// frontend/next.config.ts reads DEVPOD_SUFFIX for allowedDevOrigins, so the
// frontend container gets it.
func TestDevpodFrontendGetsTheSuffix(t *testing.T) {
	project := composeConfig(t, podEnv...)
	if got := envValue(t, project, "frontend", "DEVPOD_SUFFIX"); got != "z13.internal" {
		t.Errorf("frontend DEVPOD_SUFFIX = %q, want %q", got, "z13.internal")
	}
}

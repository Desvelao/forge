package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const androidTemplate = "../../templates/android-compose"

// requireAndroidTemplate skips the test when the template is not part of this checkout.
func requireAndroidTemplate(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(androidTemplate); err != nil {
		t.Skipf("android-compose template not available: %v", err)
	}
}

func renderAndroid(t *testing.T, vars map[string]string) map[string]string {
	t.Helper()
	requireAndroidTemplate(t)
	dest := filepath.Join(t.TempDir(), "demo-app")
	if _, err := Run(androidTemplate, load(t, androidTemplate), dest,
		Options{Vars: vars, Defaults: true, Now: fixedNow}, noPrompt{t}); err != nil {
		t.Fatal(err)
	}
	return tree(t, dest)
}

func mustContain(t *testing.T, files map[string]string, name string, want ...string) {
	t.Helper()
	got, ok := files[name]
	if !ok {
		t.Fatalf("%s was not generated", name)
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s: missing %q", name, w)
		}
	}
}

func mustNotContain(t *testing.T, files map[string]string, name string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(files[name], b) {
			t.Errorf("%s: unexpected %q", name, b)
		}
	}
}

func TestAndroidTemplateDefaults(t *testing.T) {
	f := renderAndroid(t, nil)
	mustContain(t, f, "settings.gradle.kts", `rootProject.name = "demo-app"`, `include(":core", ":app")`)
	mustContain(t, f, "app.properties", "APP_NAME=DemoApp", "APP_ID=app.example.demo_app")
	mustContain(t, f, "app/build.gradle.kts", `namespace = "app.demo_app"`, `implementation(project(":core"))`)
	mustNotContain(t, f, "app/build.gradle.kts", "libs.plugins.ksp", "hilt", "room")
	mustContain(t, f, "Makefile", "$(GRADLE) :core:test testDebugUnitTest lint")
	mustContain(t, f, "app/src/main/kotlin/app/demo_app/MainActivity.kt", "import app.demo_app.core.Greeting")
	mustContain(t, f, "core/src/main/kotlin/app/demo_app/core/Greeting.kt", "package app.demo_app.core")
	mustContain(t, f, ".github/workflows/release.yml", "${{ secrets.SIGNING_KEYSTORE_BASE64 }}") // copied, never rendered
	mustContain(t, f, "CHANGELOG.md", "## [0.1.0] - 2026-01-02")
}

func TestAndroidTemplateOptions(t *testing.T) {
	f := renderAndroid(t, map[string]string{"core_module": "false", "room": "true", "hilt": "true"})
	if _, ok := f["core/build.gradle.kts"]; ok {
		t.Error("core module should be skipped")
	}
	mustContain(t, f, "settings.gradle.kts", `include(":app")`)
	mustContain(t, f, "app/build.gradle.kts", "libs.plugins.ksp", "libs.plugins.hilt", "ksp(libs.room.compiler)", "ksp(libs.hilt.compiler)", "room.schemaLocation")
	mustNotContain(t, f, "app/build.gradle.kts", `project(":core")`)
	mustContain(t, f, "build.gradle.kts", "libs.plugins.hilt")
	mustContain(t, f, "Makefile", "$(GRADLE) testDebugUnitTest lint")
	mustContain(t, f, "app/src/main/kotlin/app/demo_app/App.kt", "@HiltAndroidApp")
	mustContain(t, f, "app/src/main/kotlin/app/demo_app/MainActivity.kt", "@AndroidEntryPoint")
	mustNotContain(t, f, "app/src/main/kotlin/app/demo_app/MainActivity.kt", "Greeting")
}

func TestAndroidTemplateRejectsBadValues(t *testing.T) {
	requireAndroidTemplate(t)
	for name, vars := range map[string]map[string]string{
		"app_id":       {"app_id": "NotAnId"},
		"app_url":      {"app_url": "ftp://x"},
		"project_name": {"project_name": "Bad_Name"},
		"app_name":     {"app_name": `say "hi"`},
	} {
		_, err := Run(androidTemplate, load(t, androidTemplate), filepath.Join(t.TempDir(), "o"),
			Options{Vars: vars, Defaults: true}, noPrompt{t})
		if err == nil || !strings.Contains(err.Error(), `variable "`+name+`"`) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

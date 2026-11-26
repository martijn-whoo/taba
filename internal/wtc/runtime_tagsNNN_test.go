package wtc

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runtimeTagsNNNFixture(t *testing.T) *Context {
	t.Helper()
	t.Setenv("TABA_RUNTIME_DECLARATIONS", "")
	c, _ := rootDekitFixture(t, map[string]string{
		"dekit.yaml": `load:
  - file: services/web.yaml
    at: web
tasks:
  device/main: {cmd: [sh, -c, ':'], tags: [lab, device, z-last, a-first]}
  plain/main: {cmd: [sh, -c, ':']}
  empty/main: {cmd: [sh, -c, ':'], tags: ~}
`,
		"services/web.yaml": "tasks:\n  tunnel: {cmd: [sh, -c, ':'], tags: [public, endpoint]}\n",
	})
	fixtureFile(t, c.runtimeLocalFragment("widget"), "tasks:\n  db/main: {cmd: [sh, -c, ':'], tags: [accessory, postgres], x-wtc: {port_env: DB_PORT}}\n", 0600)
	fixtureFile(t, runtimeSharedFragment(c.Harness), "tasks:\n  tools/main: {cmd: [sh, -c, ':'], tags: [harness]}\n", 0644)
	return c
}

var runtimeTagsNNNWant = map[string]struct {
	kind string
	tags []string
}{
	"widget/device/main": {"service", []string{"lab", "device", "z-last", "a-first"}},
	"widget/plain/main":  {"service", nil},
	"widget/empty/main":  {"service", nil},
	"widget/web/tunnel":  {"endpoint", []string{"public", "endpoint"}},
	"widget/db/main":     {"accessory", []string{"accessory", "postgres"}},
	"harness/tools/main": {"service", []string{"harness"}},
}

func runtimeTagsNNNCheck(t *testing.T, name string, items []RuntimeItem) {
	t.Helper()
	seen := 0
	for _, item := range items {
		want, ok := runtimeTagsNNNWant[item.Path]
		if !ok {
			continue
		}
		seen++
		if item.Kind != want.kind || !reflect.DeepEqual(item.Tags, want.tags) {
			t.Errorf("%s %s: kind %q tags %q, want %q %q", name, item.Path, item.Kind, item.Tags, want.kind, want.tags)
		}
	}
	if seen != len(runtimeTagsNNNWant) {
		t.Fatalf("%s: %d of %d tasks in %+v", name, seen, len(runtimeTagsNNNWant), items)
	}
}

func TestRuntimeTagsNNNAsWrittenInEveryDeclaration(t *testing.T) {
	c := runtimeTagsNNNFixture(t)
	runtimeTagsNNNCheck(t, "enlisted", c.RuntimeEnlisted())
	runtimeTagsNNNCheck(t, "status", c.RuntimeStatus())
	m, _ := rootDekitPlanTasks(t, c)
	var planned []RuntimeItem
	for _, task := range m.Tasks {
		planned = append(planned, task.RuntimeItem)
	}
	runtimeTagsNNNCheck(t, "plan", planned)
}

func TestRuntimeTagsNNNSurviveRenderAndRunnerObservation(t *testing.T) {
	c := runtimeTagsNNNFixture(t)
	if _, err := c.RenderRuntime(false); err != nil {
		t.Fatal(err)
	}
	m, err := c.runtimeManifest()
	if err != nil {
		t.Fatal(err)
	}
	var recorded []RuntimeItem
	for _, task := range m.Tasks {
		recorded = append(recorded, task.RuntimeItem)
	}
	runtimeTagsNNNCheck(t, "manifest", recorded)
	runtimeTagsNNNCheck(t, "enlisted after render", c.RuntimeEnlisted())
	// dekit's own ls adds runner tags such as autostart; the declaration's stay.
	fixtureFile(t, filepath.Join(c.runtimeRoot(), "running"), "", 0600)
	fixtureFile(t, filepath.Join(c.runtimeRoot(), "fake-tasks.json"), `{"tasks":[{"path":"widget/device/main","state":"ready","tags":["autostart","lab"]},{"path":"widget/web/tunnel","state":"failed","exit_code":3,"tags":["endpoint"]}]}`, 0600)
	got := c.RuntimeStatus()
	runtimeTagsNNNCheck(t, "live", got)
	for _, item := range got {
		if item.Path == "widget/web/tunnel" && (item.State != "failed" || item.ExitCode == nil || *item.ExitCode != 3) {
			t.Fatalf("lost live facts: %+v", item)
		}
	}
}

func TestRuntimeTagsNNNFillTasksRenderedWithoutThem(t *testing.T) {
	c := runtimeFixture(t)
	shared := runtimeSharedFragment(filepath.Join(c.Collection, "widget"))
	recorded := []RuntimeItem{
		{Path: "widget/web/tunnel", Kind: "endpoint", State: "ready", Source: shared, URL: "https://example.invalid"},
		{Path: "widget/web/server", State: "ready", Source: filepath.Join(c.Collection, "elsewhere.yaml")},
	}
	got := c.runtimeWithDeclarations(append([]RuntimeItem(nil), recorded...))
	if !reflect.DeepEqual(got[0].Tags, []string{"endpoint"}) || got[0].State != "ready" || got[0].URL != recorded[0].URL {
		t.Fatalf("task rendered before tags did not gain only its tags: %+v", got[0])
	}
	if got[1].Tags != nil {
		t.Fatalf("tags from a file that no longer declares the task: %+v", got[1])
	}
	own := []RuntimeItem{{Path: "widget/web/tunnel", Kind: "endpoint", State: "ready", Source: shared, Tags: []string{"recorded"}}}
	if got := c.runtimeWithDeclarations(own); !reflect.DeepEqual(got[0].Tags, []string{"recorded"}) {
		t.Fatalf("declaration replaced recorded tags: %+v", got[0])
	}
}

func TestRuntimeTagsNNNOnlyInJSON(t *testing.T) {
	tagged := RuntimeItem{Collection: "example", Path: "widget/web/tunnel", Kind: "endpoint", State: "ready", Tags: []string{"lab", "endpoint"}}
	plain := tagged
	plain.Tags = nil
	data, err := json.Marshal([]RuntimeItem{tagged, plain})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded[0]["tags"], []any{"lab", "endpoint"}) {
		t.Fatalf("tags in JSON: %s", data)
	}
	if _, ok := decoded[1]["tags"]; ok {
		t.Fatalf("a task without tags has a tags field: %s", data)
	}
	if RuntimeText([]RuntimeItem{tagged}) != RuntimeText([]RuntimeItem{plain}) {
		t.Fatal("tags changed the text output")
	}
}

func TestRuntimeTagsNNNRefusesWhatIsNotAListOfStrings(t *testing.T) {
	for name, tags := range map[string]string{
		"number":       "[1]",
		"boolean":      "[lab, true]",
		"null":         "[~]",
		"object":       "[{lab: true}]",
		"string":       "lab",
		"object value": "{lab: true}",
	} {
		t.Run(name, func(t *testing.T) {
			for file, write := range map[string]func(c *Context, repo string){
				"dekit.yaml": func(c *Context, repo string) {
					fixtureFile(t, filepath.Join(repo, "dekit.yaml"), "tasks: {web: {cmd: [sh, -c, ':'], tags: "+tags+"}}\n", 0644)
				},
				"svc file": func(c *Context, repo string) {
					fixtureFile(t, c.runtimeLocalFragment("widget"), "tasks: {web: {cmd: [sh, -c, ':'], tags: "+tags+"}}\n", 0600)
				},
			} {
				c, repo := rootDekitFixture(t, nil)
				write(c, repo)
				const want = "widget/web: tags must be a list of strings"
				items := c.RuntimeEnlisted()
				if len(items) != 1 || items[0].Kind != "configuration" || !strings.Contains(items[0].Error, want) {
					t.Fatalf("%s discovery %+v, want %q", file, items, want)
				}
				if _, _, err := c.runtimePlan("dekit"); err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("%s render error %v, want %q", file, err, want)
				}
			}
		})
	}
}

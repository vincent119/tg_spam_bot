package migration

import "testing"

func TestMigrationManifestStreams(t *testing.T) {
	files, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("預期八個 migration 版本，實際 %d", len(files))
	}
	streams := map[int64]string{
		20261005110000: "core", 20261005110100: "semantic",
		20261005111400: "core", 20261005111500: "semantic",
		20261005111600: "core", 20261005111700: "core", 20261005111800: "core",
		20261005133800: "core",
	}
	for _, file := range files {
		if file.Stream != streams[file.Version] || len(file.Checksum) != 64 || len(file.Tables) == 0 {
			t.Fatalf("版本 %d stream、checksum 或契約無效", file.Version)
		}
		if file.Version == 20261005133800 {
			columns := file.Tables["detection_events"].Columns
			for name, expectedType := range map[string]string{
				"username": "text", "first_name": "text", "message_sent_at": "timestamp with time zone",
			} {
				actual, ok := columns[name]
				if !ok || actual.Type != expectedType || actual.NotNull {
					t.Fatalf("133800 的 %s 欄位契約無效", name)
				}
			}
		}
	}
}

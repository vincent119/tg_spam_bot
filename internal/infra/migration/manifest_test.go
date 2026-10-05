package migration

import "testing"

func TestMigrationManifestStreams(t *testing.T) {
	files, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 7 {
		t.Fatalf("預期七個 migration 版本，實際 %d", len(files))
	}
	streams := map[int64]string{
		20261005110000: "core", 20261005110100: "semantic",
		20261005111400: "core", 20261005111500: "semantic",
		20261005111600: "core", 20261005111700: "core", 20261005111800: "core",
	}
	for _, file := range files {
		if file.Stream != streams[file.Version] || len(file.Checksum) != 64 || len(file.Tables) == 0 {
			t.Fatalf("版本 %d stream、checksum 或契約無效", file.Version)
		}
	}
}

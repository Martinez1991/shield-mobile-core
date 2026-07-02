package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzKeepClasses asserts the manifest parser never panics on malformed XML
// (issue #10). AndroidManifest.xml is attacker-influenced input.
func FuzzKeepClasses(f *testing.F) {
	f.Add("")
	f.Add("<manifest>")
	f.Add(`<manifest package="com.x"><application android:name=".A"/></manifest>`)
	f.Add(`<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="p"><application><activity android:name/></application>`)
	f.Add("<<<>>>&;")
	f.Fuzz(func(t *testing.T, data string) {
		path := filepath.Join(t.TempDir(), "AndroidManifest.xml")
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Skip()
		}
		_, _ = KeepClasses(path) // must not panic
	})
}

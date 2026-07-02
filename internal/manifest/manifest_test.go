package manifest

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const sampleManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="com.bank.app">
  <application android:name=".App">
    <activity android:name=".MainActivity"/>
    <activity android:name="com.bank.app.pay.CheckoutActivity"/>
    <activity-alias android:name=".Alias" android:targetActivity=".MainActivity"/>
    <service android:name=".sync.SyncService"/>
    <receiver android:name="BootReceiver"/>
    <provider android:name=".data.AppProvider"/>
  </application>
</manifest>`

func TestKeepClasses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AndroidManifest.xml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := KeepClasses(path)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{
		"com.bank.app.App",
		"com.bank.app.Alias",
		"com.bank.app.BootReceiver", // relative w/o dot
		"com.bank.app.MainActivity", // from activity + alias target (deduped)
		"com.bank.app.data.AppProvider",
		"com.bank.app.pay.CheckoutActivity", // fully-qualified kept as-is
		"com.bank.app.sync.SyncService",
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keep[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestKeepClassesMissingFile(t *testing.T) {
	if _, err := KeepClasses(filepath.Join(t.TempDir(), "nope.xml")); err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestResolve(t *testing.T) {
	cases := []struct{ pkg, name, want string }{
		{"com.x", ".Foo", "com.x.Foo"},
		{"com.x", "Foo", "com.x.Foo"},
		{"com.x", "com.y.Foo", "com.y.Foo"},
		{"com.x", "", ""},
	}
	for _, c := range cases {
		if got := resolve(c.pkg, c.name); got != c.want {
			t.Errorf("resolve(%q,%q)=%q want %q", c.pkg, c.name, got, c.want)
		}
	}
}

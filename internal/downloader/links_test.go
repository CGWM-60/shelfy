package downloader

import (
	"reflect"
	"testing"
)

func TestParseLinks(t *testing.T) {
	input := "\nhttps://a.test/file1\n  https://b.test/file2  \n\n"
	got := ParseLinks(input)
	want := []string{"https://a.test/file1", "https://b.test/file2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseLinks mismatch got=%v want=%v", got, want)
	}
}

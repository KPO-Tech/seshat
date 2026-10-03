package pdfsmart

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// The encrypted fixtures (see ../pdftext/testdata/encrypted/README.md) are the 3-page document of three_pages.pdf
// encrypted by pypdf, and a one-page one encrypted by MuPDF. User password "user-secret".
func encryptedFixture(t *testing.T, name string) []byte {
	t.Helper()
	return readTestdata(t, filepath.Join("encrypted", name))
}

func TestAnEncryptedPDFWithAnEmptyUserPasswordIsRead(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"aes256_nouser.pdf", "aes256r5_nouser.pdf", "aes128_nouser.pdf", "rc4_128_nouser.pdf"} {
		t.Run(name, func(t *testing.T) {
			data := encryptedFixture(t, name)
			if n, err := PageCount(data); err != nil || n != 3 {
				t.Fatalf("PageCount = %d, %v", n, err)
			}
			result, ok, err := ReadPages(context.Background(), data, []int{2}, Options{}, nil, VisionFallback{})
			if err != nil || !ok {
				t.Fatalf("ReadPages: ok=%v err=%v", ok, err)
			}
			if !strings.Contains(result.Markdown, "Bravo section") {
				t.Fatalf("page 2 = %q", result.Markdown)
			}
		})
	}
}

func TestEncryptionThatSomeProducersWriteInlineIsOpenedToo(t *testing.T) {
	t.Parallel()
	// MuPDF writes the encryption dictionary directly in the trailer instead of as an indirect object.
	data := encryptedFixture(t, "mupdf_aes256_nouser.pdf")
	if n, err := PageCount(data); err != nil || n != 1 {
		t.Fatalf("PageCount = %d, %v", n, err)
	}
}

func TestAPasswordProtectedPDFAsksForThePassword(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"aes256_user.pdf", "mupdf_aes256_user.pdf"} {
		data := encryptedFixture(t, name)
		if _, err := PageCount(data); !errors.Is(err, ErrPasswordRequired) {
			t.Errorf("%s: PageCount error = %v, want ErrPasswordRequired", name, err)
		}
		if _, _, err := ReadPages(context.Background(), data, nil, Options{}, nil, VisionFallback{}); !errors.Is(err, ErrPasswordRequired) {
			t.Errorf("%s: ReadPages error = %v, want ErrPasswordRequired", name, err)
		}
	}
}

func TestTheRightPasswordOpensItAndAWrongOneDoesNot(t *testing.T) {
	t.Parallel()
	data := encryptedFixture(t, "aes256_user.pdf")
	result, ok, err := ReadPages(context.Background(), data, []int{3}, Options{Password: "user-secret"}, nil, VisionFallback{})
	if err != nil || !ok || !strings.Contains(result.Markdown, "Charlie section") {
		t.Fatalf("right password: ok=%v err=%v md=%q", ok, err, result.Markdown)
	}
	// The owner password opens it too, as in any viewer.
	if _, _, err := ReadPages(context.Background(), data, []int{1}, Options{Password: "owner-secret"}, nil, VisionFallback{}); err != nil {
		t.Fatalf("owner password: %v", err)
	}
	if _, _, err := ReadPages(context.Background(), data, nil, Options{Password: "not-it"}, nil, VisionFallback{}); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("wrong password error = %v, want ErrPasswordRequired", err)
	}
}

func TestAFileThatIsNotEncryptedComesBackAsItWas(t *testing.T) {
	t.Parallel()
	plain := threePages(t)
	got, err := Unlock(plain, "")
	if err != nil || &got[0] != &plain[0] {
		t.Fatalf("an open PDF should be returned unchanged (err=%v)", err)
	}
	junk := []byte("this is not a pdf at all")
	if got, err := Unlock(junk, ""); err != nil || string(got) != string(junk) {
		t.Fatalf("junk should be returned for the caller to report: %v", err)
	}
}

func TestADecryptedCopyIsRememberedForTheNextCall(t *testing.T) {
	// Not parallel: the cache is shared and small, and other tests pushing their own files in would evict this one.
	data := encryptedFixture(t, "aes256_nouser.pdf")
	first, err := Unlock(data, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Unlock(data, "")
	if err != nil || &first[0] != &second[0] {
		t.Fatalf("the second call should reuse the first's copy (err=%v)", err)
	}
}

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestRenderDynamicIndexReadsLatestBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html": {Data: []byte(`<script>window.BASE_URL = "{{ .BASE_URL }}"</script><script src="assets/old.js"></script>`)},
	}

	render := func() string {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		renderDynamicIndex(ctx, files, "index.html", "/app/")
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
		}
		return recorder.Body.String()
	}

	if body := render(); !strings.Contains(body, `assets/old.js`) || !strings.Contains(body, `window.BASE_URL`) {
		t.Fatalf("unexpected first index: %q", body)
	}
	files["index.html"] = &fstest.MapFile{Data: []byte(`<script>window.BASE_URL = "{{ .BASE_URL }}"</script><script src="assets/new.js"></script>`)}
	if body := render(); !strings.Contains(body, `assets/new.js`) || strings.Contains(body, `assets/old.js`) {
		t.Fatalf("dynamic index did not pick up the latest build: %q", body)
	}
}

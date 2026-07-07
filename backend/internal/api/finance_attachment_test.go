package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/auth"
)

func TestFinanceTransactionAttachmentDownload(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "finance-attachment@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, u.ID, "Attachment", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "finance-attachment@example.com", "secret123")

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fields := map[string]string{
		"transaction_date": "2026-04-10",
		"direction":        "outgoing",
		"amount_cents":     "1234",
		"note":             "receipt",
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("attachment", "receipt.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("receipt contents")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(prop.ID, 10)+"/finance/transactions", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	createRaw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d want 201 body=%s", res.StatusCode, string(createRaw))
	}
	var created struct {
		Transaction struct {
			ID             int64  `json:"id"`
			AttachmentPath string `json:"attachment_path"`
		} `json:"transaction"`
	}
	if err := json.Unmarshal(createRaw, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Transaction.ID == 0 || created.Transaction.AttachmentPath == "" {
		t.Fatalf("created transaction missing attachment: %+v", created.Transaction)
	}

	downloadURL := ts.URL + "/api/properties/" + strconv.FormatInt(prop.ID, 10) + "/finance/transactions/" + strconv.FormatInt(created.Transaction.ID, 10) + "/attachment/download"
	dlReq, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	dlReq.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		dlReq.AddCookie(c)
	}
	dlRes, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		t.Fatal(err)
	}
	defer dlRes.Body.Close()
	dlRaw, _ := io.ReadAll(dlRes.Body)
	if dlRes.StatusCode != http.StatusOK {
		t.Fatalf("download status=%d want 200 body=%s", dlRes.StatusCode, string(dlRaw))
	}
	if string(dlRaw) != "receipt contents" {
		t.Fatalf("download body=%q", string(dlRaw))
	}
	if cd := dlRes.Header.Get("Content-Disposition"); !strings.Contains(cd, "receipt.txt") {
		t.Fatalf("Content-Disposition=%q", cd)
	}
}

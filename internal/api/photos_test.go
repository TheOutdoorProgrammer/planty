package api

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMultipartUploadKeepsCaptureTime(t *testing.T) {
	want := time.Date(2025, time.June, 4, 13, 22, 10, 125_000_000, time.UTC)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("photo", "plant.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("jpeg")); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("taken_at", want.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/v1/plants/fern/photos", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	_, _, _, got, err := readUpload(httptest.NewRecorder(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Fatalf("capture time = %s, want %s", got, want)
	}
}

func TestUploadRejectsInvalidCaptureTime(t *testing.T) {
	req := httptest.NewRequest(
		"POST",
		"/v1/plants/fern/photos?taken_at=last-tuesday",
		bytes.NewReader([]byte("jpeg")),
	)
	req.Header.Set("Content-Type", "image/jpeg")

	if _, _, _, _, err := readUpload(httptest.NewRecorder(), req); err == nil {
		t.Fatal("invalid taken_at was accepted")
	}
}

func TestMultipartIdentificationEnforcesTheFileLimit(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("photo", "plant.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("12345")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/identify", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())

	if _, _, err := readImage(httptest.NewRecorder(), req, 4); !errors.Is(err, ErrPhotoSize) {
		t.Fatalf("oversized multipart image returned %v, want ErrPhotoSize", err)
	}
}

func TestRawIdentificationEnforcesTheFileLimit(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/identify", bytes.NewReader([]byte("12345")))
	req.Header.Set("Content-Type", "image/jpeg")

	if _, _, err := readImage(httptest.NewRecorder(), req, 4); !errors.Is(err, ErrPhotoSize) {
		t.Fatalf("oversized raw image returned %v, want ErrPhotoSize", err)
	}
}

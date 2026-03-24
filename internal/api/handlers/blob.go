// Package handlers – blob.go implements the S3-compatible object storage API.
//
// Supported S3 operations:
//
//	GET    /s3/                                  – ListBuckets
//	PUT    /s3/{bucket}                          – CreateBucket
//	DELETE /s3/{bucket}                          – DeleteBucket
//	GET    /s3/{bucket}                          – ListObjects
//	PUT    /s3/{bucket}/{key+}                   – PutObject
//	GET    /s3/{bucket}/{key+}                   – GetObject
//	HEAD   /s3/{bucket}/{key+}                   – HeadObject
//	DELETE /s3/{bucket}/{key+}                   – DeleteObject
//	PUT    /s3/{bucket}/{key+}?copy-source={src} – CopyObject
package handlers

import (
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/blob"
)

// BlobHandler handles S3-compatible HTTP requests.
type BlobHandler struct {
	store  *blob.Store
	region string
}

// NewBlobHandler creates a BlobHandler.
func NewBlobHandler(store *blob.Store, region string) *BlobHandler {
	return &BlobHandler{store: store, region: region}
}

// --- S3 XML types ------------------------------------------------------------

type listBucketsResult struct {
	XMLName xml.Name         `xml:"ListAllMyBucketsResult"`
	Xmlns   string           `xml:"xmlns,attr"`
	Owner   s3Owner          `xml:"Owner"`
	Buckets []s3BucketEntry  `xml:"Buckets>Bucket"`
}

type s3Owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type s3BucketEntry struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type listObjectsResult struct {
	XMLName     xml.Name         `xml:"ListBucketResult"`
	Xmlns       string           `xml:"xmlns,attr"`
	Name        string           `xml:"Name"`
	Prefix      string           `xml:"Prefix"`
	MaxKeys     int              `xml:"MaxKeys"`
	IsTruncated bool             `xml:"IsTruncated"`
	Contents    []s3ObjectEntry  `xml:"Contents"`
}

type s3ObjectEntry struct {
	Key          string  `xml:"Key"`
	LastModified string  `xml:"LastModified"`
	ETag         string  `xml:"ETag"`
	Size         int64   `xml:"Size"`
	StorageClass string  `xml:"StorageClass"`
}

// --- ListBuckets: GET /s3/ ---------------------------------------------------

func (h *BlobHandler) ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.store.ListBuckets()
	if err != nil {
		h.xmlError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	entries := make([]s3BucketEntry, len(buckets))
	for i, b := range buckets {
		entries[i] = s3BucketEntry{Name: b, CreationDate: time.Now().UTC().Format(time.RFC3339)}
	}
	result := listBucketsResult{
		Xmlns:   "http://s3.amazonaws.com/doc/2006-03-01/",
		Owner:   s3Owner{ID: "void", DisplayName: "voiddb"},
		Buckets: entries,
	}
	h.writeXML(w, http.StatusOK, result)
}

// --- CreateBucket: PUT /s3/{bucket} ------------------------------------------

func (h *BlobHandler) CreateBucket(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	if err := h.store.CreateBucket(bucket); err != nil {
		h.xmlError(w, http.StatusBadRequest, "InvalidBucketName", err.Error())
		return
	}
	w.Header().Set("Location", "/"+bucket)
	w.WriteHeader(http.StatusOK)
}

// --- DeleteBucket: DELETE /s3/{bucket} ---------------------------------------

func (h *BlobHandler) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	force := r.URL.Query().Get("force") == "true"
	if err := h.store.DeleteBucket(bucket, force); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			h.xmlError(w, http.StatusConflict, "BucketNotEmpty", err.Error())
		} else {
			h.xmlError(w, http.StatusNotFound, "NoSuchBucket", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- ListObjects: GET /s3/{bucket} -------------------------------------------

func (h *BlobHandler) ListObjects(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	prefix := r.URL.Query().Get("prefix")
	objects, err := h.store.ListObjects(bucket, prefix)
	if err != nil {
		h.xmlError(w, http.StatusNotFound, "NoSuchBucket", err.Error())
		return
	}
	entries := make([]s3ObjectEntry, len(objects))
	for i, o := range objects {
		entries[i] = s3ObjectEntry{
			Key:          o.Key,
			LastModified: o.LastModified.UTC().Format(time.RFC3339),
			ETag:         `"` + o.ETag + `"`,
			Size:         o.Size,
			StorageClass: "STANDARD",
		}
	}
	result := listObjectsResult{
		Xmlns:    "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:     bucket,
		Prefix:   prefix,
		MaxKeys:  1000,
		Contents: entries,
	}
	h.writeXML(w, http.StatusOK, result)
}

// --- PutObject: PUT /s3/{bucket}/{key+} --------------------------------------

func (h *BlobHandler) PutObject(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	key := mux.Vars(r)["key"]

	// Handle CopyObject (x-amz-copy-source header).
	if copySource := r.Header.Get("X-Amz-Copy-Source"); copySource != "" {
		parts := strings.SplitN(strings.TrimPrefix(copySource, "/"), "/", 2)
		if len(parts) != 2 {
			h.xmlError(w, http.StatusBadRequest, "InvalidArgument", "invalid copy-source")
			return
		}
		meta, err := h.store.CopyObject(parts[0], parts[1], bucket, key)
		if err != nil {
			h.xmlError(w, http.StatusInternalServerError, "InternalError", err.Error())
			return
		}
		type copyResult struct {
			XMLName      xml.Name `xml:"CopyObjectResult"`
			ETag         string   `xml:"ETag"`
			LastModified string   `xml:"LastModified"`
		}
		h.writeXML(w, http.StatusOK, copyResult{
			ETag:         `"` + meta.ETag + `"`,
			LastModified: meta.LastModified.UTC().Format(time.RFC3339),
		})
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	// Collect custom metadata from x-amz-meta-* headers.
	userMeta := make(map[string]string)
	for k, v := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-meta-") {
			userMeta[strings.TrimPrefix(lk, "x-amz-meta-")] = v[0]
		}
	}

	meta, err := h.store.PutObject(bucket, key, ct, r.Body, userMeta)
	if err != nil {
		h.xmlError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	w.WriteHeader(http.StatusOK)
}

// --- GetObject: GET /s3/{bucket}/{key+} --------------------------------------

func (h *BlobHandler) GetObject(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	key := mux.Vars(r)["key"]

	rc, meta, err := h.store.GetObject(bucket, key)
	if err != nil {
		h.xmlError(w, http.StatusNotFound, "NoSuchKey", err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	w.Header().Set("Last-Modified", meta.LastModified.UTC().Format(http.TimeFormat))
	for k, v := range meta.Metadata {
		w.Header().Set("X-Amz-Meta-"+k, v)
	}
	w.WriteHeader(http.StatusOK)
	buf := make([]byte, 32*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// --- HeadObject: HEAD /s3/{bucket}/{key+} ------------------------------------

func (h *BlobHandler) HeadObject(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	key := mux.Vars(r)["key"]

	meta, err := h.store.HeadObject(bucket, key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("ETag", `"`+meta.ETag+`"`)
	w.Header().Set("Last-Modified", meta.LastModified.UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
}

// --- DeleteObject: DELETE /s3/{bucket}/{key+} --------------------------------

func (h *BlobHandler) DeleteObject(w http.ResponseWriter, r *http.Request) {
	bucket := mux.Vars(r)["bucket"]
	key := mux.Vars(r)["key"]
	if err := h.store.DeleteObject(bucket, key); err != nil {
		h.xmlError(w, http.StatusNotFound, "NoSuchKey", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers -----------------------------------------------------------------

// writeXML serialises v to XML and writes it to w.
func (h *BlobHandler) writeXML(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(v)
}

// xmlError writes an S3-style XML error response.
func (h *BlobHandler) xmlError(w http.ResponseWriter, status int, code, message string) {
	type s3Error struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	h.writeXML(w, status, s3Error{Code: code, Message: message})
}

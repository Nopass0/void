// Package api wires all HTTP handlers and middleware into a single http.Handler.
package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/voiddb/void/internal/api/handlers"
	"github.com/voiddb/void/internal/api/middleware"
	"github.com/voiddb/void/internal/auth"
	"github.com/voiddb/void/internal/blob"
	"github.com/voiddb/void/internal/engine"
	"github.com/voiddb/void/internal/kvcache"
)

// RouterOptions configures the router.
type RouterOptions struct {
	// CORSOrigins is the list of allowed CORS origins.
	CORSOrigins []string
	// S3Region is reported in blob API responses.
	S3Region string
}

// NewRouter builds and returns the main HTTP router for VoidDB.
func NewRouter(
	store *engine.Store,
	authSvc *auth.Service,
	blobStore *blob.Store,
	cache *kvcache.Cache,
	opts RouterOptions,
) http.Handler {
	r := mux.NewRouter()

	// Let CORS middleware answer browser preflight requests for any path.
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}).Methods(http.MethodOptions)

	// Global middleware.
	r.Use(middleware.CORS(opts.CORSOrigins))
	r.Use(middleware.RequestLogger)
	r.Use(middleware.JSON)

	// Instantiate handlers.
	authH   := handlers.NewAuthHandler(authSvc)
	dbH     := handlers.NewDBHandler(store)
	blobH   := handlers.NewBlobHandler(blobStore, opts.S3Region)
	backupH := handlers.NewBackupHandler(store, "1.0.0")
	cacheH  := handlers.NewCacheHandler(cache)
	logsH   := handlers.NewLogsHandler()
	metaH   := handlers.NewMetaHandler()

	// --- Public auth routes --------------------------------------------------
	pub := r.PathPrefix("/v1/auth").Subrouter()
	pub.HandleFunc("/login", authH.Login).Methods(http.MethodPost, http.MethodOptions)
	pub.HandleFunc("/refresh", authH.Refresh).Methods(http.MethodPost, http.MethodOptions)

	// --- Protected routes (require valid JWT) --------------------------------
	api := r.PathPrefix("/v1").Subrouter()
	api.Use(middleware.Auth(authSvc))

	// Auth / user management.
	api.HandleFunc("/auth/me", authH.Me).Methods(http.MethodGet)

	// User management (admin only).
	adminOnly := api.PathPrefix("/users").Subrouter()
	adminOnly.Use(middleware.RequireRole(auth.RoleAdmin))
	adminOnly.HandleFunc("", authH.ListUsers).Methods(http.MethodGet)
	adminOnly.HandleFunc("", authH.CreateUser).Methods(http.MethodPost)
	adminOnly.HandleFunc("/{id}", authH.DeleteUser).Methods(http.MethodDelete)
	adminOnly.HandleFunc("/{id}/password", authH.ChangePassword).Methods(http.MethodPut)

	// Engine stats and logs.
	api.HandleFunc("/stats", dbH.Stats).Methods(http.MethodGet)
	api.HandleFunc("/logs", logsH.Get).Methods(http.MethodGet)
	api.HandleFunc("/logs/realtime", logsH.Realtime).Methods(http.MethodGet)

	// Backup / restore (admin only).
	backupAdmin := api.PathPrefix("/backup").Subrouter()
	backupAdmin.Use(middleware.RequireRole(auth.RoleAdmin))
	backupAdmin.HandleFunc("", backupH.Export).Methods(http.MethodPost)
	backupAdmin.HandleFunc("/restore", backupH.Restore).Methods(http.MethodPost)

	// Database management.
	api.HandleFunc("/databases", dbH.ListDatabases).Methods(http.MethodGet)
	api.HandleFunc("/databases", dbH.CreateDatabase).Methods(http.MethodPost)
	api.HandleFunc("/databases/{db}", dbH.DeleteDatabase).Methods(http.MethodDelete)
	api.HandleFunc("/databases/{db}/realtime", dbH.Realtime).Methods(http.MethodGet)
	
	// Collection management.
	api.HandleFunc("/databases/{db}/collections", dbH.ListCollections).Methods(http.MethodGet)
	api.HandleFunc("/databases/{db}/collections", dbH.CreateCollection).Methods(http.MethodPost)
	api.HandleFunc("/databases/{db}/collections/{col}", dbH.DeleteCollection).Methods(http.MethodDelete)
	
	// Schema management.
	api.HandleFunc("/databases/{db}/{col}/schema", dbH.GetSchema).Methods(http.MethodGet)
	api.HandleFunc("/databases/{db}/{col}/schema", dbH.SetSchema).Methods(http.MethodPut)

	// Document CRUD.
	api.HandleFunc("/databases/{db}/{col}/query", dbH.QueryDocuments).Methods(http.MethodPost)
	api.HandleFunc("/databases/{db}/{col}/count", dbH.CountDocuments).Methods(http.MethodGet)
	api.HandleFunc("/databases/{db}/{col}", dbH.InsertDocument).Methods(http.MethodPost)
	api.HandleFunc("/databases/{db}/{col}/{id}", dbH.GetDocument).Methods(http.MethodGet)
	api.HandleFunc("/databases/{db}/{col}/{id}", dbH.ReplaceDocument).Methods(http.MethodPut)
	api.HandleFunc("/databases/{db}/{col}/{id}", dbH.PatchDocument).Methods(http.MethodPatch)
	api.HandleFunc("/databases/{db}/{col}/{id}", dbH.DeleteDocument).Methods(http.MethodDelete)

	// Cache API.
	api.HandleFunc("/cache/{key:.*}", cacheH.Get).Methods(http.MethodGet)
	api.HandleFunc("/cache/{key:.*}", cacheH.Set).Methods(http.MethodPost)
	api.HandleFunc("/cache/{key:.*}", cacheH.Delete).Methods(http.MethodDelete)

	// --- S3-compatible blob API (separate path prefix) -----------------------
	s3 := r.PathPrefix("/s3").Subrouter()
	s3.Use(middleware.Auth(authSvc))
	s3.HandleFunc("/", blobH.ListBuckets).Methods(http.MethodGet)
	s3.HandleFunc("/{bucket}", blobH.CreateBucket).Methods(http.MethodPut)
	s3.HandleFunc("/{bucket}", blobH.DeleteBucket).Methods(http.MethodDelete)
	s3.HandleFunc("/{bucket}", blobH.ListObjects).Methods(http.MethodGet)
	s3.HandleFunc("/{bucket}/{key:.*}", blobH.PutObject).Methods(http.MethodPut)
	s3.HandleFunc("/{bucket}/{key:.*}", blobH.GetObject).Methods(http.MethodGet)
	s3.HandleFunc("/{bucket}/{key:.*}", blobH.HeadObject).Methods(http.MethodHead)
	s3.HandleFunc("/{bucket}/{key:.*}", blobH.DeleteObject).Methods(http.MethodDelete)

	// Health-check (no auth required).
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	}).Methods(http.MethodGet)
	r.HandleFunc("/skill.md", metaH.Skill).Methods(http.MethodGet)
	r.HandleFunc("/.well-known/voiddb-skill.md", metaH.Skill).Methods(http.MethodGet)

	return r
}

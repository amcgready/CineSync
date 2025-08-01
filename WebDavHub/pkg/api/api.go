package api

import (
	"cinesync/pkg/logger"
	"cinesync/pkg/db"
	"cinesync/pkg/env"
	"cinesync/pkg/config"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var rootDir string
var lastStats Stats
var lastStatsUpdate time.Time
var statsCacheDuration = 30 * time.Second
var statsScanInProgress bool
var statsScanProgress struct {
	CurrentPath string
	FilesScanned int
	FoldersScanned int
	TotalSize int64
	LastUpdate time.Time
}

var isPlaceholderConfig bool

// HTTP client for faster API requests
var httpClientWithTimeout = &http.Client{
	Timeout: 1 * time.Second,
}

// MediaHub SSE variables for real-time updates
var (
	mediaHubClients      = make(map[chan string]bool)
	mediaHubClientsMutex sync.RWMutex
)

// SetRootDir sets the root directory for file operations and initializes the DB
func SetRootDir(dir string) {
	rootDir = dir

	// Check if we're using a placeholder or fallback configuration
	originalDestDir := env.GetString("DESTINATION_DIR", "")
	isPlaceholderConfig = (originalDestDir == "" ||
		originalDestDir == "/path/to/destination" ||
		originalDestDir == "\\path\\to\\destination" ||
		dir == ".")

	if err := db.InitDB(rootDir); err != nil {
		logger.Warn("Failed to initialize SQLite DB: %v", err)
	}
	if err := db.InitTmdbCacheTable(); err != nil {
		logger.Warn("Failed to initialize TMDB cache table: %v", err)
	}
	if err := db.InitSourceDB(); err != nil {
		logger.Warn("Failed to initialize source files DB: %v", err)
	} else {
		// Set the broadcast callback to avoid circular dependency
		db.BroadcastEventCallback = BroadcastMediaHubEvent

		// Check if this is a new database and trigger initial scan
		if db.IsNewDatabase() {
			logger.Info("New source database detected, scheduling initial scan")
			go func() {
				time.Sleep(3 * time.Second) // Give the system time to fully initialize
				if err := db.ScanSourceDirectories("startup"); err != nil {
					logger.Error("Failed to perform initial scan: %v", err)
				}
			}()
		}
	}

	// Initialize folder cache for fast navigation
	if !isPlaceholderConfig {
		go func() {
			time.Sleep(2 * time.Second)
			if err := db.InitializeFolderCache(); err != nil {
				logger.Warn("Failed to initialize folder cache: %v", err)
			}
		}()
	}
}

// UpdateRootDir updates the root directory when configuration changes
func UpdateRootDir() {
	newDestDir := env.GetString("DESTINATION_DIR", ".")

	// Check if the directory exists
	if _, err := os.Stat(newDestDir); os.IsNotExist(err) {
		logger.Warn("Directory %s does not exist. Please create it manually before using it as DESTINATION_DIR", newDestDir)
		return
	}

	// Update the root directory only if it exists
	oldRootDir := rootDir
	SetRootDir(newDestDir)

	logger.Info("Root directory updated from %s to %s", oldRootDir, newDestDir)
}

// getSourceDirectories returns the configured source directories
func getSourceDirectories() []string {
	sourceDirStr := env.GetString("SOURCE_DIR", "")
	if sourceDirStr == "" {
		return []string{}
	}

	// Split by comma and clean up paths
	dirs := strings.Split(sourceDirStr, ",")
	var validDirs []string
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir != "" && dir != "/path/to/files" { // Skip placeholder values
			validDirs = append(validDirs, dir)
		}
	}
	return validDirs
}

// getProcessedFilesMap gets all processed files for a directory to avoid repeated database queries
func getProcessedFilesMap(sourceDir string) map[string]struct{} {
	processedFiles := make(map[string]struct{})

	// Try to get database connection
	mediaHubDB, err := db.GetDatabaseConnection()
	if err != nil {
		logger.Warn("Failed to get database connection for processed files check: %v", err)
		return processedFiles
	}

	// Query the processed_files table for files in this source directory
	query := `SELECT file_path FROM processed_files WHERE file_path LIKE ? AND destination_path IS NOT NULL AND destination_path != ''`
	rows, err := mediaHubDB.Query(query, sourceDir+"%")
	if err != nil {
		logger.Warn("Error querying processed files: %v", err)
		return processedFiles
	}
	defer rows.Close()

	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			continue
		}
		processedFiles[filePath] = struct{}{}
	}

	return processedFiles
}

// InitializeImageCache initializes the image cache service with project directory
func InitializeImageCache(projectDir string) {
	InitImageCache(projectDir)
}

type FileInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     string `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
	Path     string `json:"path,omitempty"`
	FullPath string `json:"fullPath,omitempty"`
	Icon     string `json:"icon,omitempty"`
	IsSeasonFolder bool `json:"isSeasonFolder,omitempty"`
	HasSeasonFolders bool `json:"hasSeasonFolders,omitempty"`
	IsCategoryFolder bool `json:"isCategoryFolder,omitempty"`
	TmdbId   string `json:"tmdbId,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	PosterPath string `json:"posterPath,omitempty"`
	Title    string `json:"title,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	FirstAirDate string `json:"firstAirDate,omitempty"`
	SeasonNumber *int `json:"seasonNumber,omitempty"`
	EpisodeNumber *int `json:"episodeNumber,omitempty"`
	IsSourceRoot bool `json:"isSourceRoot,omitempty"`
	IsSourceFile bool `json:"isSourceFile,omitempty"`
	IsMediaFile  bool `json:"isMediaFile,omitempty"`
	SourcePath   string `json:"sourcePath,omitempty"`
	DestinationPath string `json:"destinationPath,omitempty"`
}

type Stats struct {
	TotalFiles   int    `json:"totalFiles"`
	TotalFolders int    `json:"totalFolders"`
	TotalSize    string `json:"totalSize"`
	LastSync     string `json:"lastSync"`
	WebDAVStatus string `json:"webdavStatus"`
	StorageUsed  string `json:"storageUsed"`
	IP           string `json:"ip"`
	Port         string `json:"port"`
	TotalMovies  int    `json:"totalMovies"`
	TotalShows   int    `json:"totalShows"`
}

type ReadlinkRequest struct {
	Path string `json:"path"`
}

type ReadlinkResponse struct {
	RealPath      string `json:"realPath"`
	AbsPath       string `json:"absPath"`
	Error         string `json:"error,omitempty"`
	FileSize      *int64 `json:"fileSize,omitempty"`
	FormattedSize string `json:"formattedSize,omitempty"`
	TmdbID        string `json:"tmdbId,omitempty"`
	SeasonNumber  *int   `json:"seasonNumber,omitempty"`
	FoundInDB     bool   `json:"foundInDB"`
}

type DeleteRequest struct {
	Path  string   `json:"path"`
	Paths []string `json:"paths"`
}

type DeleteResponse struct {
	Success      bool     `json:"success"`
	Error        string   `json:"error,omitempty"`
	DeletedCount int      `json:"deletedCount,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type RenameRequest struct {
	OldPath string `json:"oldPath"`
	NewName string `json:"newName"`
}

type RenameResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// getFileIcon returns a string representing the icon type for a file
func getFileIcon(name string, isDir bool) string {
	if isDir {
		return "folder"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".webm", ".avi", ".mov", ".mkv":
		return "movie"
	case ".mp3", ".wav", ".ogg", ".flac":
		return "music"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return "image"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".txt", ".rtf":
		return "text"
	case ".xls", ".xlsx", ".csv":
		return "spreadsheet"
	case ".ppt", ".pptx":
		return "presentation"
	case ".zip", ".rar", ".tar", ".gz", ".7z":
		return "archive"
	case ".go", ".js", ".html", ".css", ".py", ".java", ".c", ".cpp", ".php", ".rb":
		return "code"
	default:
		return "file"
	}
}

// HandleHealth returns a simple health check response for MediaHub dashboard availability checking
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status": "ok",
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleConfigStatus returns the configuration status
func HandleConfigStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the current destination directory from environment (which may have been reloaded)
	currentDestDir := env.GetString("DESTINATION_DIR", ".")

	// Check if the current destination directory is a placeholder or invalid
	currentIsPlaceholder := currentDestDir == "/path/to/destination" ||
		currentDestDir == "\\path\\to\\destination" ||
		currentDestDir == "." ||
		currentDestDir == ""

	// Also check if the directory actually exists
	if !currentIsPlaceholder {
		if _, err := os.Stat(currentDestDir); os.IsNotExist(err) {
			logger.Warn("DESTINATION_DIR %s does not exist", currentDestDir)
			currentIsPlaceholder = true
		}
	}

	response := map[string]interface{}{
		"isPlaceholder":        currentIsPlaceholder,
		"destinationDir":       currentDestDir,
		"effectiveRootDir":     rootDir,
		"needsConfiguration":   currentIsPlaceholder,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getTmdbDataFromCacheByID(tmdbID string, mediaType string) (string, string, string, string, string) {
	if tmdbID == "" {
		return "", "", "", "", ""
	}

	// Try multiple media type variations to handle mixed database values
	mediaTypeVariations := []string{
		strings.ToLower(mediaType),
		mediaType,                 
		strings.ToUpper(mediaType),
	}

	// For TV shows, also try common variations
	if strings.ToLower(mediaType) == "tv" || strings.ToLower(mediaType) == "tvshow" {
		mediaTypeVariations = append(mediaTypeVariations, "tv", "TV", "tvshow", "tvShow", "TvShow")
	}

	for _, mt := range mediaTypeVariations {
		cacheKey := "id:" + tmdbID + ":" + mt

		if result, err := db.GetTmdbCache(cacheKey); err == nil && result != "" {
			var tmdbData map[string]interface{}
			if err := json.Unmarshal([]byte(result), &tmdbData); err == nil {
				posterPath := ""
				title := ""
				resultMediaType := ""
				releaseDate := ""
				firstAirDate := ""

				if pp, ok := tmdbData["poster_path"].(string); ok {
					posterPath = pp
				}
				if t, ok := tmdbData["title"].(string); ok {
					title = t
				}
				if mt, ok := tmdbData["media_type"].(string); ok {
					resultMediaType = mt
				}
				if rd, ok := tmdbData["release_date"].(string); ok {
					releaseDate = rd
				}
				if fad, ok := tmdbData["first_air_date"].(string); ok {
					firstAirDate = fad
				}

				if posterPath != "" {
					return posterPath, title, resultMediaType, releaseDate, firstAirDate
				}
			}
		}
	}

	return "", "", "", "", ""
}

func getTmdbDataFromCache(folderName string) (string, string, string, string, string) {
	cacheKeys := []string{
		folderName + "||movie",
		folderName + "||tv",
		folderName + "||",
		folderName,
		strings.ToLower(folderName) + "||movie",
		strings.ToLower(folderName) + "||tv",
		strings.ToLower(folderName) + "||",
		strings.ToLower(folderName),
	}

	for _, cacheKey := range cacheKeys {
		if result, err := db.GetTmdbCache(cacheKey); err == nil && result != "" {
			var tmdbData map[string]interface{}
			if err := json.Unmarshal([]byte(result), &tmdbData); err == nil {
				posterPath := ""
				title := ""
				mediaType := ""
				releaseDate := ""
				firstAirDate := ""

				if pp, ok := tmdbData["poster_path"].(string); ok {
					posterPath = pp
				}
				if t, ok := tmdbData["title"].(string); ok {
					title = t
				}
				if mt, ok := tmdbData["media_type"].(string); ok {
					mediaType = mt
				}
				if rd, ok := tmdbData["release_date"].(string); ok {
					releaseDate = rd
				}
				if fad, ok := tmdbData["first_air_date"].(string); ok {
					firstAirDate = fad
				}

				return posterPath, title, mediaType, releaseDate, firstAirDate
			}
		}
	}

	return "", "", "", "", ""
}

func resolveActualDirectoryPath(requestedDir, apiPath string) (string, error) {
	if _, err := os.Stat(requestedDir); err == nil {
		return requestedDir, nil
	}

	pathParts := strings.Split(strings.Trim(apiPath, "/"), "/")
	if len(pathParts) == 0 {
		return requestedDir, nil
	}

	currentPath := rootDir
	for _, part := range pathParts {
		if part == "" {
			continue
		}

		nextPath := filepath.Join(currentPath, part)
		if _, err := os.Stat(nextPath); err == nil {
			currentPath = nextPath
			continue
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return requestedDir, nil
		}

		found := false
		idPattern := regexp.MustCompile(`\s*\{(?:tmdb|imdb|tvdb)-[^}]+\}`)
		for _, entry := range entries {
			if entry.IsDir() {
				baseName := strings.TrimSpace(idPattern.ReplaceAllString(entry.Name(), ""))
				if baseName == part {
					currentPath = filepath.Join(currentPath, entry.Name())
					found = true
					break
				}
			}
		}

		if !found {
			return requestedDir, nil
		}
	}

	return currentPath, nil
}

func HandleFiles(w http.ResponseWriter, r *http.Request) {
	logger.Info("Request: %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If using placeholder configuration, return a special response
	if isPlaceholderConfig {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Needs-Configuration", "true")
		json.NewEncoder(w).Encode([]FileInfo{})
		return
	}

	path := r.URL.Path
	if path == "/api/files" {
		path = "/"
	} else {
		path = strings.TrimPrefix(path, "/api/files")
	}

	// Parse pagination parameters
	page := 1
	limit := 100
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	// Parse search parameter
	searchQuery := strings.TrimSpace(r.URL.Query().Get("search"))

	letterFilter := strings.TrimSpace(r.URL.Query().Get("letter"))

	dir := filepath.Join(rootDir, path)

	actualDir, err := resolveActualDirectoryPath(dir, path)
	if err != nil {
		logger.Warn("Failed to resolve directory path: %s - %v", dir, err)
		http.Error(w, "Failed to resolve directory path", http.StatusInternalServerError)
		return
	}
	dir = actualDir

	logger.Info("Listing directory: %s (API path: %s)", dir, path)

	// Try database-first approach for folder listing with pagination
	var dbFolders []db.FolderInfo
	var totalDbFolders int
	var dbErr error
	var useDatabase bool

	if searchQuery == "" && letterFilter == "" {
		// Regular folder listing - use cached database approach
		dbFolders, totalDbFolders, dbErr = db.GetFoldersFromDatabaseCached(path, page, limit)
		if dbErr != nil {
			useDatabase = false
		} else {
			useDatabase = len(dbFolders) > 0
		}
	} else if searchQuery != "" {
		dbFolders, totalDbFolders, dbErr = db.SearchFoldersFromDatabase(path, searchQuery, page, limit)
		if dbErr != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Total-Count", "0")
			w.Header().Set("X-Page", fmt.Sprintf("%d", page))
			w.Header().Set("X-Limit", fmt.Sprintf("%d", limit))
			w.Header().Set("X-Total-Pages", "0")
			w.Header().Set("X-Search-Query", searchQuery)
			json.NewEncoder(w).Encode([]FileInfo{})
			return
		} else {
			useDatabase = true
		}
	} else if letterFilter != "" {
		// Use database search with letter filtering for better performance
		dbFolders, totalDbFolders, dbErr = db.SearchFoldersFromDatabaseWithLetter(path, letterFilter, page, limit)
		if dbErr != nil {
			dbFolders, totalDbFolders, dbErr = db.GetFoldersFromDatabaseCached(path, 1, 10000)
			useDatabase = dbErr == nil && len(dbFolders) > 0
		} else {
			useDatabase = true
		}
	}

	// Only read filesystem if database is not available AND it's not a search query
	var entries []os.DirEntry
	if !useDatabase && searchQuery == "" {
		var err error
		entries, err = os.ReadDir(dir)
		if err != nil {
			logger.Warn("Failed to read directory: %s - %v", dir, err)
			http.Error(w, "Failed to read directory", http.StatusInternalServerError)
			return
		}
	} else {
		entries = []os.DirEntry{}
	}

	// Process all entries
	var tmdbID string
	var mediaType string

	folderName := filepath.Base(dir)
	isCurrentDirCategoryFolder := isCategoryFolderFromDB(folderName)

	if useDatabase {
		if !isCurrentDirCategoryFolder && len(dbFolders) > 0 {
			tmdbID = dbFolders[0].TmdbID
			mediaType = dbFolders[0].MediaType

		}
	}

	effectiveEntries := entries

	files := make([]FileInfo, 0)
	seasonFolderCount := 0
	fileCount := 0

	// Create a map to track which folders we've already processed from database
	processedFolders := make(map[string]bool)

	// First, add folders from database
	if len(dbFolders) > 0 {
		for _, dbFolder := range dbFolders {
			fileInfo := FileInfo{
				Name:     dbFolder.FolderName,
				Type:     "directory",
				Path:     dbFolder.FolderPath,
				FullPath: dbFolder.FolderPath,
				Icon:     getFileIcon(dbFolder.FolderName, true),
				Modified: dbFolder.Modified,
			}

			// Set database metadata
			if dbFolder.TmdbID != "" {
				fileInfo.TmdbId = dbFolder.TmdbID
			}
			if dbFolder.MediaType != "" {
				fileInfo.MediaType = dbFolder.MediaType
				if dbFolder.MediaType == "tv" || dbFolder.MediaType == "TV" {
					fileInfo.HasSeasonFolders = true
				}
			}

			// Get poster path from TMDB cache using database metadata
			if dbFolder.TmdbID != "" && dbFolder.MediaType != "" {
				posterPath, title, _, releaseDate, firstAirDate := getTmdbDataFromCacheByID(dbFolder.TmdbID, dbFolder.MediaType)
				if posterPath != "" {
					fileInfo.PosterPath = posterPath
					// Use clean title from TMDB cache (consistent with subdirectory handling)
					fileInfo.Title = title
					if releaseDate != "" {
						fileInfo.ReleaseDate = releaseDate
					}
					if firstAirDate != "" {
						fileInfo.FirstAirDate = firstAirDate
					}
				}
			}

			// Check if it's a season folder
			if isSeasonFolder(dbFolder.FolderName) {
				fileInfo.IsSeasonFolder = true
				seasonFolderCount++
			}

			// Check if it's a category folder
			if isCategoryFolderFromDB(dbFolder.FolderName) {
				fileInfo.IsCategoryFolder = true
			}

			files = append(files, fileInfo)
			processedFolders[dbFolder.FolderName] = true
		}
	}

	// Check for allowed extensions in this directory
	allowedExtStr := os.Getenv("ALLOWED_EXTENSIONS")
	allowedExts := []string{}
	if allowedExtStr != "" {
		for _, ext := range strings.Split(allowedExtStr, ",") {
			ext = strings.TrimSpace(strings.ToLower(ext))
			if ext != "" {
				if !strings.HasPrefix(ext, ".") {
					ext = "." + ext
				}
				allowedExts = append(allowedExts, ext)
			}
		}
	}
	hasAllowed := false
	// Check for allowed extensions in this directory only
	if len(allowedExts) > 0 {
		for _, entry := range effectiveEntries {
			if !entry.IsDir() {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
						for _, allowed := range allowedExts {
							if ext == allowed {
								hasAllowed = true
								break
							}
						}
						if hasAllowed {
							break
						}
					}
				}
	}

	w.Header().Set("X-Has-Allowed-Extensions", fmt.Sprintf("%v", hasAllowed))
	// Set TMDB headers if we have the information AND it's not a category folder
	if tmdbID != "" && path != "/" && !isCurrentDirCategoryFolder {
		w.Header().Set("X-TMDB-ID", tmdbID)
		if mediaType != "" {
			w.Header().Set("X-Media-Type", mediaType)
		}
	}

	// --- TV Show Root Detection ---
	if len(effectiveEntries) > 0 {
		seasonCount := 0
		fileCountInDir := 0
		for _, entry := range effectiveEntries {
			if entry.IsDir() && isSeasonFolder(entry.Name()) {
				seasonCount++
			} else if !entry.IsDir() {
				fileCountInDir++
			}
		}
		if seasonCount > 0 && seasonCount == len(effectiveEntries)-fileCountInDir && fileCountInDir == 0 {
			w.Header().Set("X-Has-Season-Folders", "true")
			if mediaType == "" { // Only set if not already determined from database
				w.Header().Set("X-Media-Type", "tv")
				mediaType = "tv" // Update local mediaType as well
				logger.Info("Directory %s identified as TV Show root by content, X-Media-Type set to tv", dir)
			}
		}
	}

	for _, entry := range effectiveEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip directories that were already processed from database
		if entry.IsDir() && processedFolders[entry.Name()] {
			continue
		}

		filePath := path
		if !strings.HasSuffix(filePath, "/") {
			filePath += "/"
		}
		filePath += entry.Name()

		fileInfo := FileInfo{
			Name:     entry.Name(),
			Type:     "file",
			Modified: info.ModTime().Format(time.RFC3339),
			Path:     filePath,
			FullPath: filePath,
			Icon:     getFileIcon(entry.Name(), entry.IsDir()),
		}

		if entry.IsDir() {
			fileInfo.Type = "directory"
			if isSeasonFolder(entry.Name()) {
				fileInfo.IsSeasonFolder = true
				seasonFolderCount++
			}

			// --- Subdirectory TMDB/Media Type Logic ---
			subDirTmdbID := ""
			subDirMediaType := ""

			// Check if it's a category folder
			isSubdirCategoryFolder := isCategoryFolderFromDB(entry.Name())
			if isSubdirCategoryFolder {
				fileInfo.IsCategoryFolder = true
			}

			// Get TMDB info from database if using database mode
			if !isSubdirCategoryFolder && useDatabase {
				for _, dbFolder := range dbFolders {
					if dbFolder.FolderName == entry.Name() {
						subDirTmdbID = dbFolder.TmdbID
						subDirMediaType = dbFolder.MediaType
						fileInfo.TmdbId = subDirTmdbID
						fileInfo.MediaType = subDirMediaType
						if subDirMediaType == "tv" || subDirMediaType == "TV" {
							fileInfo.HasSeasonFolders = true
						}
						break
					}
				}
			}

			var posterPath, title, cachedMediaType, releaseDate, firstAirDate string

			// Only get poster data if it's not a category folder
			if !isSubdirCategoryFolder {
				if subDirTmdbID != "" && subDirMediaType != "" {
					posterPath, title, cachedMediaType, releaseDate, firstAirDate = getTmdbDataFromCacheByID(subDirTmdbID, subDirMediaType)
				}

				if posterPath == "" && subDirTmdbID != "" {
					posterPath, title, cachedMediaType, releaseDate, firstAirDate = getTmdbDataFromCacheByID(subDirTmdbID, "movie")
					if posterPath == "" {
						posterPath, title, cachedMediaType, releaseDate, firstAirDate = getTmdbDataFromCacheByID(subDirTmdbID, "tv")
					}
				}

				if posterPath == "" {
					posterPath, title, cachedMediaType, releaseDate, firstAirDate = getTmdbDataFromCache(entry.Name())
				}
			}

			if posterPath != "" {
				fileInfo.PosterPath = posterPath
				fileInfo.Title = title
				if releaseDate != "" {
					fileInfo.ReleaseDate = releaseDate
				}
				if firstAirDate != "" {
					fileInfo.FirstAirDate = firstAirDate
				}
				if subDirMediaType == "" && cachedMediaType != "" {
					fileInfo.MediaType = cachedMediaType
					subDirMediaType = cachedMediaType
					if cachedMediaType == "tv" {
						fileInfo.HasSeasonFolders = true
					}
				}
			}

			// For season folders, try to inherit TMDB ID from parent directory
			if isSeasonFolder(entry.Name()) && subDirTmdbID == "" {
				// Check if parent directory has TMDB data
				if tmdbID != "" {
					fileInfo.TmdbId = tmdbID
					fileInfo.MediaType = "tv" // Season folders are always TV
					logger.Info("Season folder %s inherited TMDB ID %s from parent directory", entry.Name(), tmdbID)
				}
			}

			logger.Info("Found directory: %s", filePath)
		} else {
			// Try to get comprehensive file info from database first, fallback to filesystem
			fullFilePath := filepath.Join(dir, entry.Name())
			if dbInfo, found := db.GetFileInfoFromDatabase(fullFilePath); found && dbInfo.FileSize > 0 {
				fileInfo.Size = formatFileSize(dbInfo.FileSize)
				fileInfo.SourcePath = dbInfo.SourcePath
				fileInfo.DestinationPath = dbInfo.DestinationPath
				if dbInfo.TmdbID != "" {
					fileInfo.TmdbId = dbInfo.TmdbID
				}
				if dbInfo.SeasonNumber > 0 {
					fileInfo.SeasonNumber = &dbInfo.SeasonNumber
				}
				if dbInfo.EpisodeNumber > 0 {
					fileInfo.EpisodeNumber = &dbInfo.EpisodeNumber
				}
			} else {
				fileInfo.Size = formatFileSize(info.Size())
			}
			fileCount++
		}

		files = append(files, fileInfo)
	}

	// Apply search filtering if search query is provided
	if searchQuery != "" {
		var filteredFiles []FileInfo
		searchLower := strings.ToLower(searchQuery)
		for _, file := range files {
			if strings.Contains(strings.ToLower(file.Name), searchLower) {
				filteredFiles = append(filteredFiles, file)
			}
		}
		files = filteredFiles
	}

	if letterFilter != "" {
		var filteredFiles []FileInfo
		isNumeric := letterFilter == "#"
		lowerLetter := strings.ToLower(letterFilter)

		for _, file := range files {
			firstChar := file.Name[:1]
			if isNumeric {
				if len(firstChar) > 0 && firstChar[0] >= '0' && firstChar[0] <= '9' {
					filteredFiles = append(filteredFiles, file)
				}
			} else {
				if strings.ToLower(firstChar) == lowerLetter {
					filteredFiles = append(filteredFiles, file)
				}
			}
		}
		files = filteredFiles
	}

	// Sort files: directories first, then alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].Type == "directory" && files[j].Type != "directory" {
			return true
		}
		if files[i].Type != "directory" && files[j].Type == "directory" {
			return false
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	var totalFiles int
	if useDatabase && (searchQuery != "" || letterFilter != "") {
		// For database searches (including letter filtering), use the total count from database
		totalFiles = totalDbFolders
	} else if useDatabase && searchQuery == "" && letterFilter == "" {
		// For regular database listing without filters
		totalFiles = totalDbFolders
	} else {
		// For filesystem-based results, apply pagination manually
		totalFiles = len(files)
		startIndex := (page - 1) * limit
		endIndex := startIndex + limit

		if startIndex >= totalFiles {
			files = []FileInfo{}
		} else {
			if endIndex > totalFiles {
				endIndex = totalFiles
			}
			files = files[startIndex:endIndex]
		}
	}


	w.Header().Set("X-Total-Count", fmt.Sprintf("%d", totalFiles))
	w.Header().Set("X-Page", fmt.Sprintf("%d", page))
	w.Header().Set("X-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-Total-Pages", fmt.Sprintf("%d", (totalFiles+limit-1)/limit))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// HandleSourceFiles handles requests for browsing source directories
func HandleSourceFiles(w http.ResponseWriter, r *http.Request) {
	logger.Info("Request: %s %s", r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get source directories from configuration
	sourceDirs := getSourceDirectories()
	logger.Info("Source directories configured: %v", sourceDirs)
	if len(sourceDirs) == 0 {
		logger.Warn("No source directories configured")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Needs-Configuration", "true")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "No source directories configured",
			"data": []FileInfo{},
		})
		return
	}

	path := r.URL.Path
	if path == "/api/source-browse" {
		path = "/"
	} else {
		path = strings.TrimPrefix(path, "/api/source-browse")
	}

	// Parse pagination parameters
	page := 1
	limit := 100
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if parsedPage, err := strconv.Atoi(pageStr); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	// Parse search parameter
	searchQuery := strings.TrimSpace(r.URL.Query().Get("search"))

	letterFilter := strings.TrimSpace(r.URL.Query().Get("letter"))

	// Parse source directory index (for multiple source dirs)
	sourceIndex := 0
	if indexStr := r.URL.Query().Get("source"); indexStr != "" {
		if parsedIndex, err := strconv.Atoi(indexStr); err == nil && parsedIndex >= 0 && parsedIndex < len(sourceDirs) {
			sourceIndex = parsedIndex
		}
	}

	// If path is root, show source directories list
	if path == "/" && sourceIndex == 0 && len(sourceDirs) > 1 {
		var files []FileInfo
		for i, sourceDir := range sourceDirs {
			if stat, err := os.Stat(sourceDir); err == nil {
				// Get directory name for better display
				dirName := filepath.Base(sourceDir)

				files = append(files, FileInfo{
					Name:     dirName,
					Type:     "directory",
					Size:     "",
					Modified: stat.ModTime().Format("2006-01-02 15:04:05"),
					FullPath: fmt.Sprintf("/?source=%d", i),
					Path:     sourceDir, // Store full path for reference
					IsSourceRoot: true,
				})
			} else {
				logger.Warn("Source directory %s is not accessible: %v", sourceDir, err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Source-Directories", strings.Join(sourceDirs, ","))
		json.NewEncoder(w).Encode(files)
		return
	}

	// If only one source directory or specific source selected, browse it directly
	if path == "/" && len(sourceDirs) == 1 {
		sourceIndex = 0
	}

	// Browse specific source directory
	sourceDir := sourceDirs[sourceIndex]

	// Clean the path and handle Windows paths properly
	cleanPath := strings.ReplaceAll(path, "/", string(filepath.Separator))
	if cleanPath == string(filepath.Separator) {
		cleanPath = ""
	}

	dir := filepath.Join(sourceDir, cleanPath)
	logger.Info("Listing source directory: %s (API path: %s, sourceDir: %s, cleanPath: %s)", dir, path, sourceDir, cleanPath)

	// Security check - ensure we're within the source directory
	absSourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		logger.Warn("Failed to get absolute source directory path: %v", err)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		logger.Warn("Failed to get absolute directory path: %v", err)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(absDir, absSourceDir) {
		logger.Warn("Path outside source directory: %s", absDir)
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("Failed to read source directory: %s - %v", dir, err)
		http.Error(w, fmt.Sprintf("Failed to read directory: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Info("Found %d entries in source directory: %s", len(entries), dir)

	// Get processed files map for this source directory to filter them out
	processedFiles := getProcessedFilesMap(sourceDir)

	var files []FileInfo
	allowedExts := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".3gp", ".ogv"}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Skip hidden files and directories
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Use forward slashes for API paths regardless of OS
		apiPath := path
		if apiPath == "/" {
			apiPath = "/" + entry.Name()
		} else {
			apiPath = apiPath + "/" + entry.Name()
		}

		// Get the full file system path for processing status check
		fullFilePath := filepath.Join(sourceDir, cleanPath, entry.Name())

		// Skip files that have already been processed (have symlinks created)
		if _, isProcessed := processedFiles[fullFilePath]; isProcessed && !entry.IsDir() {
			logger.Info("Skipping already processed file: %s", fullFilePath)
			continue
		}

		fileInfo := FileInfo{
			Name:     entry.Name(),
			Type:     "file",
			Size:     formatFileSize(info.Size()),
			Modified: info.ModTime().Format("2006-01-02 15:04:05"),
			FullPath: fmt.Sprintf("%s?source=%d", apiPath, sourceIndex),
			Path:     apiPath,
			IsSourceFile: true,
		}

		if entry.IsDir() {
			fileInfo.Type = "directory"
			fileInfo.Size = ""
			logger.Info("Found source directory: %s", apiPath)
		} else {
			// Check if it's a media file
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			isMediaFile := false
			for _, allowed := range allowedExts {
				if ext == allowed {
					isMediaFile = true
					break
				}
			}
			fileInfo.IsMediaFile = isMediaFile
			logger.Info("Found source file: %s (Size: %s, Media: %v)", apiPath, fileInfo.Size, isMediaFile)
		}

		files = append(files, fileInfo)
	}

	// Apply search filtering if search query is provided
	if searchQuery != "" {
		var filteredFiles []FileInfo
		searchLower := strings.ToLower(searchQuery)
		for _, file := range files {
			if strings.Contains(strings.ToLower(file.Name), searchLower) {
				filteredFiles = append(filteredFiles, file)
			}
		}
		files = filteredFiles
	}

	if letterFilter != "" {
		var filteredFiles []FileInfo
		isNumeric := letterFilter == "#"
		lowerLetter := strings.ToLower(letterFilter)

		for _, file := range files {
			firstChar := file.Name[:1]
			if isNumeric {
				if len(firstChar) > 0 && firstChar[0] >= '0' && firstChar[0] <= '9' {
					filteredFiles = append(filteredFiles, file)
				}
			} else {
				if strings.ToLower(firstChar) == lowerLetter {
					filteredFiles = append(filteredFiles, file)
				}
			}
		}
		files = filteredFiles
	}

	// Sort files: directories first, then alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].Type == "directory" && files[j].Type != "directory" {
			return true
		}
		if files[i].Type != "directory" && files[j].Type == "directory" {
			return false
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// Apply pagination after filtering
	totalFiles := len(files)
	startIndex := (page - 1) * limit
	endIndex := startIndex + limit

	if startIndex >= totalFiles {
		files = []FileInfo{}
	} else {
		if endIndex > totalFiles {
			endIndex = totalFiles
		}
		files = files[startIndex:endIndex]
	}

	// Add pagination headers
	w.Header().Set("X-Total-Count", fmt.Sprintf("%d", totalFiles))
	w.Header().Set("X-Page", fmt.Sprintf("%d", page))
	w.Header().Set("X-Limit", fmt.Sprintf("%d", limit))
	w.Header().Set("X-Total-Pages", fmt.Sprintf("%d", (totalFiles+limit-1)/limit))
	w.Header().Set("X-Source-Index", fmt.Sprintf("%d", sourceIndex))
	w.Header().Set("X-Source-Directory", sourceDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func isSeasonFolder(name string) bool {
	nameLower := strings.ToLower(name)
	return strings.HasPrefix(nameLower, "season ") && len(nameLower) > 7 && isNumeric(nameLower[7:])
}

func getCategoryFolders() map[string]bool {
	folders := make(map[string]bool)

	// Add configured folder names from environment variables
	for _, def := range config.GetConfigDefinitions() {
		if strings.Contains(def.Category, "Folder") || strings.Contains(def.Key, "_FOLDER") {
			if value := os.Getenv(def.Key); value != "" {
				folders[strings.ToLower(value)] = true
			}
		}
	}
	folders[strings.ToLower("CineSync")] = true

	// Add source directory names when USE_SOURCE_STRUCTURE is enabled
	if env.GetString("USE_SOURCE_STRUCTURE", "false") == "true" {
		sourceDirs := getSourceDirectories()
		for _, sourceDir := range sourceDirs {
			baseName := filepath.Base(sourceDir)
			if baseName != "" && baseName != "." && baseName != "/" {
				folders[strings.ToLower(baseName)] = true
			}
		}
	}

	return folders
}

func isCategoryFolder(folderName string) bool {
	return getCategoryFolders()[strings.ToLower(folderName)]
}

// Cache for category folders to avoid repeated database queries
var (
	categoryFoldersCache map[string]bool
	categoryFoldersMutex sync.RWMutex
	categoryFoldersExpiry time.Time
	categoryFoldersCacheDuration = 5 * time.Minute
)

// getCategoryFoldersFromDB gets all category folders from the database base_path field with caching
func getCategoryFoldersFromDB() map[string]bool {
	categoryFoldersMutex.RLock()
	if categoryFoldersCache != nil && time.Now().Before(categoryFoldersExpiry) {
		defer categoryFoldersMutex.RUnlock()
		return categoryFoldersCache
	}
	categoryFoldersMutex.RUnlock()

	categoryFoldersMutex.Lock()
	defer categoryFoldersMutex.Unlock()

	// Double-check after acquiring write lock
	if categoryFoldersCache != nil && time.Now().Before(categoryFoldersExpiry) {
		return categoryFoldersCache
	}

	folders := make(map[string]bool)

	// First add traditional category folders
	for folderName := range getCategoryFolders() {
		folders[folderName] = true
	}

	// Then add categories from database base_path field
	mediaHubDB, err := db.GetDatabaseConnection()
	if err != nil {
		return folders
	}

	// Check if base_path column exists before querying
	var dummy sql.NullString
	err = mediaHubDB.QueryRow("SELECT base_path FROM processed_files LIMIT 1").Scan(&dummy)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return folders
	}

	// Get all unique base_path values
	query := `SELECT DISTINCT base_path FROM processed_files WHERE base_path IS NOT NULL AND base_path != ''`
	rows, err := mediaHubDB.Query(query)
	if err != nil {
		return folders
	}
	defer rows.Close()

	for rows.Next() {
		var basePath string
		if err := rows.Scan(&basePath); err != nil {
			continue
		}

		parts := strings.Split(basePath, string(filepath.Separator))
		for _, part := range parts {
			if part != "" {
				folders[strings.ToLower(part)] = true
			}
		}
	}

	// Cache the results
	categoryFoldersCache = folders
	categoryFoldersExpiry = time.Now().Add(categoryFoldersCacheDuration)

	return folders
}

// isCategoryFolderFromDB checks if a folder is a category folder using database-driven logic
func isCategoryFolderFromDB(folderName string) bool {
	categoryFolders := getCategoryFoldersFromDB()
	return categoryFolders[strings.ToLower(folderName)]
}

func extractTitleFromFolderName(folderName string) string {
	re := regexp.MustCompile(`\s*\(\d{4}\)$`)
	return strings.TrimSpace(re.ReplaceAllString(folderName, ""))
}

func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func statsChanged(a, b Stats) bool {
	return a.TotalFiles != b.TotalFiles ||
		a.TotalFolders != b.TotalFolders ||
		a.TotalSize != b.TotalSize ||
		a.LastSync != b.LastSync ||
		a.WebDAVStatus != b.WebDAVStatus ||
		a.StorageUsed != b.StorageUsed ||
		a.IP != b.IP ||
		a.Port != b.Port ||
		a.TotalMovies != b.TotalMovies ||
		a.TotalShows != b.TotalShows
}

func HandleStats(w http.ResponseWriter, r *http.Request) {
	// Note: JWT is only required if CINESYNC_AUTH_ENABLED is true (handled by middleware)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check for force refresh parameter
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	// Return cached stats if they're still valid (unless force refresh requested)
	if !forceRefresh && !lastStatsUpdate.IsZero() && time.Since(lastStatsUpdate) < statsCacheDuration {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lastStats)
		return
	}

	// If a scan is already in progress, return current progress
	if statsScanInProgress {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"scanning": true,
			"progress": statsScanProgress,
		})
		return
	}

	// Start new scan
	statsScanInProgress = true
	statsScanProgress = struct {
		CurrentPath string
		FilesScanned int
		FoldersScanned int
		TotalSize int64
		LastUpdate time.Time
	}{
		CurrentPath: "Initializing database query...",
		LastUpdate: time.Now(),
	}

	// Get all stats from MediaHub database - no file system scanning needed
	totalFiles, totalFolders, totalSize, movieCount, showCount, err := db.GetAllStatsFromDB()

	if err != nil {
		// Set reasonable defaults
		totalFiles, totalFolders, totalSize, movieCount, showCount = 0, 0, 0, 0, 0
	}

	// Update progress to show completion
	statsScanProgress.FilesScanned = totalFiles
	statsScanProgress.FoldersScanned = totalFolders
	statsScanProgress.TotalSize = totalSize
	statsScanProgress.CurrentPath = "Database query completed"
	statsScanProgress.LastUpdate = time.Now()

	// For lastSync, use current time since we're not scanning files
	lastSync := time.Now()

	ip := os.Getenv("CINESYNC_IP")
	if ip == "" {
		ip = "0.0.0.0"
	}
	port := os.Getenv("CINESYNC_API_PORT")
	if port == "" {
		port = "8082"
	}
	webdavStatus := "Active"
	stats := Stats{
		TotalFiles:   totalFiles,
		TotalFolders: totalFolders,
		TotalSize:    formatFileSize(totalSize),
		LastSync:     lastSync.Format(time.RFC3339),
		WebDAVStatus: webdavStatus,
		StorageUsed:  formatFileSize(totalSize),
		IP:           ip,
		Port:         port,
		TotalMovies:  movieCount,
		TotalShows:   showCount,
	}

	if statsChanged(stats, lastStats) {
		lastStats = stats
		lastStatsUpdate = time.Now()
	}

	statsScanInProgress = false

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func HandleAuthTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If we reach here, the authentication middleware has already validated the credentials
	w.WriteHeader(http.StatusOK)
}

func HandleAuthEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	enabled := true
	if v := os.Getenv("CINESYNC_AUTH_ENABLED"); v == "false" || v == "0" {
		enabled = false
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": enabled})
}

func executeReadlink(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func HandleReadlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ReadlinkRequest
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	absPath := filepath.Join(rootDir, req.Path)
	realPath, err := executeReadlink(absPath)
	resp := ReadlinkResponse{
		RealPath: realPath,
		AbsPath:  absPath,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	// Try to get additional file information from database
	if realPath != "" {
		if dbInfo, found := db.GetFileInfoFromDatabase(realPath); found {
			resp.FileSize = &dbInfo.FileSize
			resp.FormattedSize = formatFileSize(dbInfo.FileSize)
			resp.TmdbID = dbInfo.TmdbID
			if dbInfo.SeasonNumber > 0 {
				resp.SeasonNumber = &dbInfo.SeasonNumber
			}
			resp.FoundInDB = true
			logger.Debug("Found database info for %s: size=%d, tmdb=%s", realPath, dbInfo.FileSize, dbInfo.TmdbID)
		} else {
			// Fallback: try with absPath if realPath lookup failed
			if dbInfo, found := db.GetFileInfoFromDatabase(absPath); found {
				resp.FileSize = &dbInfo.FileSize
				resp.FormattedSize = formatFileSize(dbInfo.FileSize)
				resp.TmdbID = dbInfo.TmdbID
				if dbInfo.SeasonNumber > 0 {
					resp.SeasonNumber = &dbInfo.SeasonNumber
				}
				resp.FoundInDB = true
				logger.Debug("Found database info for %s: size=%d, tmdb=%s", absPath, dbInfo.FileSize, dbInfo.TmdbID)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleDelete deletes a file or directory at the given relative path
func HandleDelete(w http.ResponseWriter, r *http.Request) {
	logger.Info("Request: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		logger.Warn("Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Warn("Error: failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req DeleteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Warn("Error: invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Handle bulk deletion if paths array is provided
	if len(req.Paths) > 0 {
		handleBulkDelete(w, req.Paths)
		return
	}

	// Handle single file deletion
	handleSingleDelete(w, req.Path)
}

// handleSingleDelete handles deletion of a single file
func handleSingleDelete(w http.ResponseWriter, relativePath string) {
	if relativePath == "" {
		logger.Warn("Error: empty path provided")
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	var path string
	var absPath string

	// Check if the path is absolute
	if filepath.IsAbs(relativePath) {
		destDir := env.GetString("DESTINATION_DIR", "")
		if destDir == "" {
			logger.Warn("Error: DESTINATION_DIR not configured for absolute path")
			http.Error(w, "DESTINATION_DIR not configured", http.StatusBadRequest)
			return
		}

		absDestDir, err := filepath.Abs(destDir)
		if err != nil {
			logger.Warn("Error: failed to get absolute DESTINATION_DIR path: %v", err)
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		reqAbsPath, err := filepath.Abs(relativePath)
		if err != nil {
			logger.Warn("Error: failed to get absolute path for request: %v", err)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Check if the absolute path is within DESTINATION_DIR
		if !strings.HasPrefix(reqAbsPath, absDestDir) {
			logger.Warn("Error: absolute path outside DESTINATION_DIR: %s", reqAbsPath)
			http.Error(w, "Path outside DESTINATION_DIR", http.StatusBadRequest)
			return
		}

		path = relativePath
		absPath = reqAbsPath
		logger.Info("Using absolute path from DESTINATION_DIR: %s", path)
	} else {
		// For relative paths, use the existing logic with rootDir
		cleanPath := filepath.Clean(relativePath)
		if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "..") {
			logger.Warn("Error: invalid relative path: %s", cleanPath)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		path = filepath.Join(rootDir, cleanPath)

		var err error
		absPath, err = filepath.Abs(path)
		if err != nil {
			logger.Warn("Error: failed to get absolute path: %v", err)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		absRoot, err := filepath.Abs(rootDir)
		if err != nil {
			logger.Warn("Error: failed to get absolute root path: %v", err)
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		if !strings.HasPrefix(absPath, absRoot) {
			logger.Warn("Error: relative path outside root directory: %s", absPath)
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Warn("Error: file or directory not found: %s", path)
		http.Error(w, "File or directory not found", http.StatusNotFound)
		return
	}

	err := os.RemoveAll(path)
	if err != nil {
		logger.Warn("Error: failed to delete %s: %v", path, err)
		http.Error(w, "Failed to delete file or directory", http.StatusInternalServerError)
		return
	}

	// Also delete from database if the record exists
	deleteFromDatabase(relativePath)

	// Clean up empty parent directories and .tmdb files
	cleanupEmptyDirectories(path)

	logger.Info("Success: deleted %s", path)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DeleteResponse{Success: true})
}

// handleBulkDelete handles deletion of multiple files
func handleBulkDelete(w http.ResponseWriter, paths []string) {
	if len(paths) == 0 {
		logger.Warn("Error: no paths provided for bulk deletion")
		http.Error(w, "No paths provided", http.StatusBadRequest)
		return
	}

	var deletedCount int
	var errors []string

	for _, relativePath := range paths {
		if relativePath == "" {
			errors = append(errors, "Empty path provided")
			continue
		}

		var path string
		var absPath string

		// Check if the path is absolute
		if filepath.IsAbs(relativePath) {
			destDir := env.GetString("DESTINATION_DIR", "")
			if destDir == "" {
				errors = append(errors, fmt.Sprintf("DESTINATION_DIR not configured for absolute path: %s", relativePath))
				continue
			}

			absDestDir, err := filepath.Abs(destDir)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to get absolute DESTINATION_DIR path for %s: %v", relativePath, err))
				continue
			}

			reqAbsPath, err := filepath.Abs(relativePath)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to get absolute path for %s: %v", relativePath, err))
				continue
			}

			// Check if the absolute path is within DESTINATION_DIR
			if !strings.HasPrefix(reqAbsPath, absDestDir) {
				errors = append(errors, fmt.Sprintf("Absolute path outside DESTINATION_DIR: %s", reqAbsPath))
				continue
			}

			path = relativePath
			absPath = reqAbsPath
		} else {
			// For relative paths, use the existing logic with rootDir
			cleanPath := filepath.Clean(relativePath)
			if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "..") {
				errors = append(errors, fmt.Sprintf("Invalid relative path: %s", cleanPath))
				continue
			}

			path = filepath.Join(rootDir, cleanPath)

			var err error
			absPath, err = filepath.Abs(path)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to get absolute path for %s: %v", relativePath, err))
				continue
			}

			absRoot, err := filepath.Abs(rootDir)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to get absolute root path for %s: %v", relativePath, err))
				continue
			}

			if !strings.HasPrefix(absPath, absRoot) {
				errors = append(errors, fmt.Sprintf("Relative path outside root directory: %s", absPath))
				continue
			}
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("File or directory not found: %s", path))
			continue
		}

		err := os.RemoveAll(path)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to delete %s: %v", path, err))
			continue
		}

		// Also delete from database if the record exists
		deleteFromDatabase(relativePath)

		// Clean up empty parent directories
		cleanupEmptyDirectories(path)

		logger.Info("Success: deleted %s", path)
		deletedCount++
	}

	// Determine overall success
	success := deletedCount > 0
	if len(errors) > 0 && deletedCount == 0 {
		success = false
	}

	response := DeleteResponse{
		Success:      success,
		DeletedCount: deletedCount,
	}

	if len(errors) > 0 {
		response.Errors = errors
		if deletedCount == 0 {
			response.Error = fmt.Sprintf("Failed to delete any files. %d errors occurred.", len(errors))
		} else {
			response.Error = fmt.Sprintf("Deleted %d files with %d errors.", deletedCount, len(errors))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// deleteFromDatabase removes a file record from the MediaHub database if it exists
func deleteFromDatabase(filePath string) {
	mediaHubDB, err := db.GetDatabaseConnection()
	if err != nil {
		logger.Warn("Failed to get database connection for cleanup: %v", err)
		return
	}

	// Try to delete the record from processed_files table
	deleteQuery := `DELETE FROM processed_files WHERE file_path = ? OR destination_path = ?`
	result, err := mediaHubDB.Exec(deleteQuery, filePath, filePath)
	if err != nil {
		logger.Warn("Failed to delete database record for %s: %v", filePath, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logger.Warn("Failed to get rows affected for database cleanup: %v", err)
		return
	}

	if removeErr := db.RemoveRecentMediaByPath(filePath); removeErr != nil {
		logger.Warn("Failed to remove recent media for path %s: %v", filePath, removeErr)
	}

	if rowsAffected > 0 {
		// Notify about the database change
		db.NotifyDashboardStatsChanged()
		db.NotifyFileOperationChanged()
	}
}

// cleanupEmptyDirectories removes empty parent directories
func cleanupEmptyDirectories(deletedPath string) {
	parentDir := filepath.Dir(deletedPath)

	for {
		// Check if this directory is empty
		isEmpty, err := isDirectoryEmpty(parentDir)
		if err != nil {
			logger.Warn("Failed to check if directory is empty: %s, error: %v", parentDir, err)
			break
		}

		// If directory has files, stop cleanup
		if !isEmpty {
			break
		}

		// If directory is empty, delete it
		if isEmpty {
			if err := os.Remove(parentDir); err != nil {
				logger.Warn("Failed to remove empty directory %s: %v", parentDir, err)
				break
			}
			logger.Info("Removed empty directory: %s", parentDir)
		}

		// Move up to the parent directory
		nextParent := filepath.Dir(parentDir)

		// Stop if we've reached the root or if we're not making progress
		if nextParent == parentDir || nextParent == "." || nextParent == "/" {
			break
		}

		parentDir = nextParent
	}
}

// isDirectoryEmpty checks if a directory is empty
func isDirectoryEmpty(dirPath string) (isEmpty bool, err error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false, err
	}

	return len(entries) == 0, nil
}

// HandleRename renames a file or directory at the given relative path
func HandleRename(w http.ResponseWriter, r *http.Request) {
	logger.Info("Request: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		logger.Warn("Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Warn("Error: failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req RenameRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Warn("Error: invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.OldPath == "" || req.NewName == "" {
		logger.Warn("Error: missing oldPath or newName")
		http.Error(w, "oldPath and newName are required", http.StatusBadRequest)
		return
	}

	cleanOldPath := filepath.Clean(req.OldPath)
	if cleanOldPath == "." || cleanOldPath == ".." || strings.HasPrefix(cleanOldPath, "..") {
		logger.Warn("Error: invalid oldPath: %s", cleanOldPath)
		http.Error(w, "Invalid oldPath", http.StatusBadRequest)
		return
	}

	oldFullPath := filepath.Join(rootDir, cleanOldPath)
	newFullPath := filepath.Join(filepath.Dir(oldFullPath), req.NewName)

	absOld, err := filepath.Abs(oldFullPath)
	if err != nil {
		logger.Warn("Error: failed to get absolute old path: %v", err)
		http.Error(w, "Invalid oldPath", http.StatusBadRequest)
		return
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		logger.Warn("Error: failed to get absolute root path: %v", err)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(absOld, absRoot) {
		logger.Warn("Error: oldPath outside root directory: %s", absOld)
		http.Error(w, "Invalid oldPath", http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(oldFullPath); os.IsNotExist(err) {
		logger.Warn("Error: file or directory not found: %s", oldFullPath)
		http.Error(w, "File or directory not found", http.StatusNotFound)
		return
	}

	if _, err := os.Stat(newFullPath); err == nil {
		logger.Warn("Error: target already exists: %s", newFullPath)
		http.Error(w, "Target already exists", http.StatusConflict)
		return
	}

	err = os.Rename(oldFullPath, newFullPath)
	if err != nil {
		logger.Warn("Error: failed to rename %s to %s: %v", oldFullPath, newFullPath, err)
		http.Error(w, "Failed to rename file or directory", http.StatusInternalServerError)
		return
	}

	logger.Info("Success: renamed %s to %s", oldFullPath, newFullPath)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RenameResponse{Success: true})
}

// HandleFileDetails handles GET/POST/DELETE for file details
func HandleFileDetails(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// GET /api/file-details?path=... or ?prefix=...
		path := r.URL.Query().Get("path")
		prefix := r.URL.Query().Get("prefix")
		if path != "" {
			fd, err := db.GetFileDetail(path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if fd == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(fd)
			return
		}
		if prefix != "" {
			fds, err := db.ListFileDetails(prefix)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(fds)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	case http.MethodPost:
		// POST /api/file-details (body: FileDetail)
		var fd db.FileDetail
		if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if err := db.UpsertFileDetail(fd); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodDelete:
		// DELETE /api/file-details?path=...
		path := r.URL.Query().Get("path")
		if path == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := db.DeleteFileDetail(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- TMDB Cache API ---
// GET /api/tmdb-cache?query=...  |  POST /api/tmdb-cache {query, result}
func HandleTmdbCache(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cacheKey := r.URL.Query().Get("query")
		if cacheKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		result, err := db.GetTmdbCache(cacheKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result == "" {
			// --- Secondary lookup for id-based cacheKey ---
			if strings.HasPrefix(cacheKey, "id:") {
				parts := strings.Split(cacheKey, ":")
				if len(parts) >= 3 {
					tmdbID := parts[1]
					mediaType := parts[2]
					// Try to find any cache row with this tmdb_id and media_type
					altResult, err := db.GetTmdbCacheByTmdbIdAndType(tmdbID, mediaType)
					if err == nil && altResult != "" {
						// Upsert under the new cacheKey for future hits
						db.UpsertTmdbCache(cacheKey, altResult)
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("X-TMDB-Cache", "HIT-SECONDARY")
						w.Write([]byte(altResult))
						return
					}
				}
			}
			// Cache miss: call TMDB API, store, and return
			// Parse cacheKey: query|year|mediaType
			parts := strings.Split(cacheKey, "|")
			query := ""
			year := ""
			mediaType := ""
			if len(parts) > 0 {
				query = parts[0]
			}
			if len(parts) > 1 {
				year = parts[1]
			}
			if len(parts) > 2 {
				mediaType = parts[2]
			}
			// Call TMDB API (proxy)
			backendHost := os.Getenv("CINESYNC_API_HOST")
			if backendHost == "" {
				backendHost = "http://localhost:8082"
			}
			params := url.Values{}
			params.Set("query", query)
			params.Set("include_adult", "false")
			if year != "" {
				params.Set("year", year)
			}
			if mediaType != "" {
				params.Set("mediaType", mediaType)
			}
			tmdbUrl := backendHost + "/api/tmdb/search?" + params.Encode()
			req, _ := http.NewRequest("GET", tmdbUrl, nil)
			req.Header = r.Header
			resp, err := httpClientWithTimeout.Do(req)
			if err != nil || resp.StatusCode != 200 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			defer resp.Body.Close()
			var tmdbResp struct {
				Results []map[string]interface{} `json:"results"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&tmdbResp); err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if len(tmdbResp.Results) == 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			best := tmdbResp.Results[0]
			id, _ := best["id"].(float64)
			title, _ := best["title"].(string)
			if title == "" {
				title, _ = best["name"].(string)
			}
			posterPath, _ := best["poster_path"].(string)
			releaseDate, _ := best["release_date"].(string)
			if releaseDate == "" {
				releaseDate, _ = best["first_air_date"].(string)
			}
			mediaType, _ = best["media_type"].(string)
			if mediaType == "" {
				parsedType := strings.ToLower(parts[len(parts)-1])
				if parsedType == "tv" {
					mediaType = "tv"
				} else {
					mediaType = "movie"
				}
			}
			resultJson := fmt.Sprintf(`{"id":%d,"title":%q,"poster_path":%q,"release_date":%q,"media_type":%q}`,
				int(id), title, posterPath, releaseDate, mediaType)
			db.UpsertTmdbCache(cacheKey, resultJson)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-TMDB-Cache", "MISS")
			w.Write([]byte(resultJson))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-TMDB-Cache", "HIT")
		w.Write([]byte(result))
		return
	case http.MethodPost:
		var req struct {
			Query  string `json:"query"`
			Result string `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Query == "" || req.Result == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := db.UpsertTmdbCache(req.Query, req.Result); err != nil {
			http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
			logger.Warn("TMDB cache upsert error: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodDelete:
		if err := db.ClearTmdbCache(); err != nil {
			http.Error(w, "Failed to clear cache: "+err.Error(), http.StatusInternalServerError)
			logger.Warn("TMDB cache clear error: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleMediaHubMessage handles structured messages and broadcasts real-time updates
func HandleMediaHubMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var message struct {
		Type      string                 `json:"type"`
		Timestamp float64                `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if message.Type == "symlink_created" {
		handleSymlinkCreated(message.Data)
	}

	forwardToPythonBridge(message)

	// Broadcast real-time update to all connected SSE clients
	broadcastMediaHubUpdate(message)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func forwardToPythonBridge(message struct {
	Type      string                 `json:"type"`
	Timestamp float64                `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}) {
	// Get the active response writer to forward the message
	activePythonResponseMutex.Lock()
	responseWriter := activePythonResponseWriter
	activePythonResponseMutex.Unlock()

	if responseWriter == nil {
		return
	}

	// Create the structured message in the format expected by the frontend
	structuredMsg := StructuredMessage{
		Type:      message.Type,
		Timestamp: message.Timestamp,
		Data:      message.Data,
	}

	// Create response with structured data
	response := PythonBridgeResponse{
		StructuredData: &structuredMsg,
	}

	// Send the response to the active bridge session
	data, err := json.Marshal(response)
	if err != nil {
		logger.Warn("Failed to marshal structured message for forwarding: %v", err)
		return
	}

	// Write to the active bridge response writer
	responseWriter.Write(data)
	responseWriter.Write([]byte("\n"))

	if flusher, ok := responseWriter.(http.Flusher); ok {
		flusher.Flush()
	}

	logger.Info("Forwarded structured message to Python bridge: %s", message.Type)
}

func handleSymlinkCreated(data map[string]interface{}) {
	mediaName, _ := data["media_name"].(string)
	mediaType, _ := data["media_type"].(string)
	destinationFile, _ := data["destination_file"].(string)
	filename, _ := data["filename"].(string)
	sourceFile, _ := data["source_file"].(string)
	tmdbIdInterface := data["tmdb_id"]
	forceMode, _ := data["force_mode"].(bool)

	var tmdbId string
	if tmdbIdInterface != nil {
		if id, ok := tmdbIdInterface.(float64); ok {
			tmdbId = fmt.Sprintf("%.0f", id)
		} else if id, ok := tmdbIdInterface.(string); ok {
			tmdbId = id
		}
	}

	if mediaName == "" || mediaType == "" {
		return
	}

	// Handle cache updates for force mode
	if forceMode {
		handleForceModeSymlinkCreated(data, tmdbId)
	}

	// Immediately update source file processing status if source file is provided
	if sourceFile != "" {
		// Extract season number if it's a TV show
		var seasonNumber *int
		if seasonInterface, exists := data["season_number"]; exists {
			if season, ok := seasonInterface.(float64); ok {
				seasonInt := int(season)
				seasonNumber = &seasonInt
			}
		}

		// Update the source file status to "processed"
		err := db.UpdateSourceFileProcessingStatus(sourceFile, "processed", tmdbId, seasonNumber)
		if err != nil {
			logger.Error("Failed to update source file processing status: %v", err)
		} else {
			logger.Info("Updated source file processing status for: %s", sourceFile)

			// Broadcast file processing event for real-time UI updates
			BroadcastMediaHubEvent("file_processed", map[string]interface{}{
				"source_file":      sourceFile,
				"destination_file": destinationFile,
				"media_name":       mediaName,
				"media_type":       mediaType,
				"tmdb_id":          tmdbId,
				"season_number":    seasonNumber,
				"filename":         filename,
			})
		}
	}

	// Determine folder name based on media type
	folderName := "Movies"
	if mediaType == "tvshow" || mediaType == "tv" {
		folderName = "TV Shows"
	}

	// Initialize the new media entry
	newMedia := db.RecentMedia{
		Name:       mediaName,
		Path:       destinationFile,
		FolderName: folderName,
		UpdatedAt:  time.Now().Unix(),
		Type:       mediaType,
		TmdbId:     tmdbId,
		Filename:   filename,
	}

	// For TV shows, use rich data directly from MediaHub
	if mediaType == "tvshow" || mediaType == "tv" {
		if seasonInterface, exists := data["season_number"]; exists {
			if season, ok := seasonInterface.(float64); ok {
				newMedia.SeasonNumber = int(season)
			} else if season, ok := seasonInterface.(int); ok {
				newMedia.SeasonNumber = season
			}
		}

		if episodeInterface, exists := data["episode_number"]; exists {
			if episode, ok := episodeInterface.(float64); ok {
				newMedia.EpisodeNumber = int(episode)
			} else if episode, ok := episodeInterface.(int); ok {
				newMedia.EpisodeNumber = episode
			}
		}

		// Use show name from MediaHub
		if showNameInterface, exists := data["show_name"]; exists {
			if showName, ok := showNameInterface.(string); ok && showName != "" {
				newMedia.ShowName = showName
			}
		}
		if newMedia.ShowName == "" {
			if properShowNameInterface, exists := data["proper_show_name"]; exists {
				if properShowName, ok := properShowNameInterface.(string); ok && properShowName != "" {
					tmdbPattern := regexp.MustCompile(`\s*\{tmdb-\d+\}`)
					cleanShowName := tmdbPattern.ReplaceAllString(properShowName, "")
					newMedia.ShowName = strings.TrimSpace(cleanShowName)
				}
			}
		}

		// Episode title (directly from MediaHub/TMDB)
		if episodeTitleInterface, exists := data["episode_title"]; exists {
			if episodeTitle, ok := episodeTitleInterface.(string); ok && episodeTitle != "" {
				seasonEpisodePattern := regexp.MustCompile(`^S\d{2}E\d{2}\s*-?\s*`)
				cleanEpisodeTitle := seasonEpisodePattern.ReplaceAllString(episodeTitle, "")
				newMedia.EpisodeTitle = strings.TrimSpace(cleanEpisodeTitle)
			}
		}
	}

	// Add to database
	if err := db.AddRecentMedia(newMedia); err != nil {
		return
	}

	// Notify dashboard about stats change (file was added)
	db.NotifyDashboardStatsChanged()
}

// handleForceModeSymlinkCreated handles cache updates for force mode symlink creation
func handleForceModeSymlinkCreated(data map[string]interface{}, tmdbId string) {
	destinationFile, _ := data["destination_file"].(string)
	mediaName, _ := data["media_name"].(string)
	mediaType, _ := data["media_type"].(string)

	var properName, year string
	if strings.Contains(mediaName, "(") && strings.Contains(mediaName, ")") {
		parts := strings.Split(mediaName, "(")
		if len(parts) >= 2 {
			properName = strings.TrimSpace(parts[0])
			yearPart := strings.Split(parts[1], ")")[0]
			year = strings.TrimSpace(yearPart)
		}
	} else {
		properName = mediaName
		year = ""
	}

	// Get season number if available
	seasonNumber := 0
	if seasonInterface, exists := data["season_number"]; exists {
		if season, ok := seasonInterface.(float64); ok {
			seasonNumber = int(season)
		}
	}

	// For force mode, invalidate the cache for the affected category to ensure consistency
	// This handles cases where the old entry might have different metadata
	if destinationFile != "" {
		destDir := env.GetString("DESTINATION_DIR", "")
		if destDir != "" {
			relativePath := strings.TrimPrefix(destinationFile, destDir)
			relativePath = strings.Trim(relativePath, "/\\")
			pathParts := strings.Split(relativePath, string(filepath.Separator))
			if len(pathParts) > 0 {
				category := pathParts[0]
				db.InvalidateFolderCacheForCategory(category)
			}
		}
	}

	if destinationFile != "" && properName != "" {
		db.UpdateFolderCacheForNewFile(destinationFile, properName, year, tmdbId, mediaType, seasonNumber)
	}
}

// broadcastMediaHubUpdate sends real-time updates to all connected SSE clients
func broadcastMediaHubUpdate(message struct {
	Type      string                 `json:"type"`
	Timestamp float64                `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}) {
	mediaHubClientsMutex.RLock()
	defer mediaHubClientsMutex.RUnlock()

	if len(mediaHubClients) == 0 {
		return
	}

	// Create SSE message
	sseMessage := map[string]interface{}{
		"type":      message.Type,
		"timestamp": message.Timestamp,
		"data":      message.Data,
	}

	messageData, err := json.Marshal(sseMessage)
	if err != nil {
		logger.Warn("Failed to marshal MediaHub SSE message: %v", err)
		return
	}

	sseData := fmt.Sprintf("data: %s\n\n", string(messageData))

	// Send to all connected clients
	for client := range mediaHubClients {
		select {
		case client <- sseData:
		default:
		}
	}
}

// BroadcastMediaHubEvent is an exported wrapper for broadcasting MediaHub events
func BroadcastMediaHubEvent(eventType string, data map[string]interface{}) {
	message := struct {
		Type      string                 `json:"type"`
		Timestamp float64                `json:"timestamp"`
		Data      map[string]interface{} `json:"data"`
	}{
		Type:      eventType,
		Timestamp: float64(time.Now().UnixMilli()),
		Data:      data,
	}

	broadcastMediaHubUpdate(message)
}

// subscribeToMediaHubUpdates adds a client to the MediaHub SSE broadcast list
func subscribeToMediaHubUpdates() chan string {
	mediaHubClientsMutex.Lock()
	defer mediaHubClientsMutex.Unlock()

	client := make(chan string, 10)
	mediaHubClients[client] = true
	return client
}

// unsubscribeFromMediaHubUpdates removes a client from the MediaHub SSE broadcast list
func unsubscribeFromMediaHubUpdates(client chan string) {
	mediaHubClientsMutex.Lock()
	defer mediaHubClientsMutex.Unlock()

	delete(mediaHubClients, client)
	close(client)
}

// HandleMediaHubEvents provides Server-Sent Events for MediaHub real-time updates
func HandleMediaHubEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to MediaHub notifications
	notificationCh := subscribeToMediaHubUpdates()
	defer unsubscribeFromMediaHubUpdates(notificationCh)

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"timestamp\":%f}\n\n", float64(time.Now().Unix()))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Keep connection alive and send messages
	for {
		select {
		case message := <-notificationCh:
			fmt.Fprint(w, message)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

// HandleRecentMedia returns the recent media list from database with dynamic episode support
func HandleRecentMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get recent media from database
	recentMedia, err := db.GetRecentMedia(10)
	if err != nil {
		http.Error(w, "Failed to retrieve recent media", http.StatusInternalServerError)
		return
	}

	// Get MediaHub database connection to look up base_path
	mediaHubDB, err := db.GetDatabaseConnection()
	if err != nil {
		logger.Warn("Failed to get MediaHub database connection for base_path lookup: %v", err)
	}

	// Convert database format to API format for compatibility
	// Initialize as empty slice to ensure JSON encodes as [] not null
	var result []map[string]interface{}
	for _, media := range recentMedia {
		item := map[string]interface{}{
			"name":       media.Name,
			"path":       media.Path,
			"folderName": media.FolderName,
			"updatedAt":  time.Unix(media.UpdatedAt, 0).Format(time.RFC3339),
			"type":       media.Type,
			"filename":   media.Filename,
		}

		// Look up additional info from processed_files table
		if mediaHubDB != nil && media.Path != "" {
			var basePath, properName, year string
			query := `SELECT COALESCE(base_path, ''), COALESCE(proper_name, ''), COALESCE(year, '') FROM processed_files WHERE destination_path = ? LIMIT 1`
			if err := mediaHubDB.QueryRow(query, media.Path).Scan(&basePath, &properName, &year); err == nil {
				if basePath != "" {
					item["basePath"] = strings.ReplaceAll(basePath, "\\", "/")
				}
				if properName != "" {
					item["properName"] = properName
				}
				if year != "" {
					item["year"] = year
				}
			}
		}

		if media.TmdbId != "" {
			item["tmdbId"] = media.TmdbId
		}
		if media.ShowName != "" {
			item["showName"] = media.ShowName
		}
		if media.SeasonNumber > 0 {
			item["seasonNumber"] = media.SeasonNumber
		}
		if media.EpisodeNumber > 0 {
			item["episodeNumber"] = media.EpisodeNumber
		}
		if media.EpisodeTitle != "" {
			item["episodeTitle"] = media.EpisodeTitle
		}

		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleRestart handles server restart requests
func HandleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logger.Info("Server restart requested")

	// Send success response before restarting
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Server restart initiated",
	})

	// Flush the response to ensure it's sent
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Give a moment for the response to be sent
	go func() {
		time.Sleep(1 * time.Second)
		logger.Info("Initiating server restart...")

		// Cross-platform graceful shutdown
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		c <- os.Interrupt

		time.Sleep(5 * time.Second)
		logger.Warn("Graceful shutdown timeout, forcing exit")
		os.Exit(0)
	}()
}
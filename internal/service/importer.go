// Package service implements the media import pipeline for fyom.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/google/uuid"
)

// Importer handles asynchronous NFO-based media library imports.
type Importer struct {
	fs          ImportFS
	providerID  string
	libraryID   string
	libraryType string
	mediaRepo   *repository.MediaRepository
	jobRepo     *repository.ImportJobRepository
	db          *repository.DB
}

// NewImporter creates a new Importer.
func NewImporter(fs ImportFS, providerID string, db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository) *Importer {
	return &Importer{
		fs:         fs,
		providerID: providerID,
		libraryID:  "default",
		mediaRepo:  mediaRepo,
		jobRepo:    jobRepo,
		db:         db,
	}
}

// SetLibraryID sets the library ID for imported items.
func (imp *Importer) SetLibraryID(id string) {
	if id != "" {
		imp.libraryID = id
	}
}

// ImportLibrary synchronously imports all media items from the library's
// source directory. Returns an ImportSummary with counts and warnings.
// This is the primary import entry point for admin-triggered scans.
func (imp *Importer) ImportLibrary(ctx context.Context, libraryID string) (*model.ImportSummary, error) {
	imp.SetLibraryID(libraryID)

	// Look up the library to get its source path and type.
	var sourcePath string
	var libType string
	err := imp.db.QueryRowContext(ctx,
		"SELECT source_path, type FROM libraries WHERE id = ?", libraryID,
	).Scan(&sourcePath, &libType)
	if err != nil {
		return nil, &errors.AppError{Code: 404, Message: "library not found"}
	}
	imp.libraryType = libType

	startTime := time.Now()

	// Stage 1: Build filesystem snapshot
	rootNode, err := imp.buildSnapshot(ctx, sourcePath)
	if err != nil {
		return nil, err
	}

	// Stage 2: Classify nodes into typed candidates
	walkCtx := &WalkContext{
		ScanID:       uuid.New().String(),
		LibraryID:    libraryID,
		SourceRoot:   sourcePath,
		ClaimedPaths: make(map[string]PathClaim),
	}
	classResult := imp.classifyTree(ctx, rootNode, walkCtx)

	// Stage 3: Parse metadata by candidate type
	imp.parseCandidateMetadata(ctx, classResult.Candidates)

	// Stage 4: Reconcile candidates into DB writes
	reconResult, err := imp.reconcileCandidates(ctx, classResult.Candidates)

	// Build summary
	return imp.buildImportSummary(startTime, classResult, reconResult, err), nil
}

func (imp *Importer) buildImportSummary(startTime time.Time, classResult *ClassificationResult, reconResult *ReconcileResult, reconErr error) *model.ImportSummary {
	summary := &model.ImportSummary{}
	if classResult != nil {
		summary.ScannedFiles = classResult.ScannedDirs
	}
	if reconResult != nil {
		for _, r := range reconResult.Accepted {
			switch r.Action {
			case ReconcileCreate:
				summary.ImportedItems++
			case ReconcileUpdate:
				summary.UpdatedItems++
			}
		}
		summary.SkippedFiles = len(reconResult.Rejected)
		summary.ParseWarnings = append(summary.ParseWarnings, imp.buildRejectWarnings(reconResult.Rejected)...)
	}
	if reconErr != nil {
		summary.ParseWarnings = append(summary.ParseWarnings, fmt.Sprintf("reconcile error: %v", reconErr))
	}
	summary.Duration = time.Since(startTime)
	return summary
}

// ImportRequest triggers an asynchronous import.
func (imp *Importer) ImportRequest(ctx context.Context, sourcePath string) (*model.ImportJob, error) {
	if _, err := imp.fs.ReadDir(ctx, sourcePath); err != nil {
		return nil, &errors.AppError{Code: 404, Message: "directory not found"}
	}

	job, err := imp.jobRepo.Create(ctx, sourcePath, imp.libraryID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	go imp.runImport(job.ID, sourcePath)

	return job, nil
}

// runImport does the actual directory scanning and DB insertion in a goroutine.
func (imp *Importer) runImport(jobID, sourcePath string) {
	ctx := context.Background()
	startTime := time.Now()

	_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "running")

	// Stage 1: Build filesystem snapshot
	rootNode, err := imp.buildSnapshot(ctx, sourcePath)
	if err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, fmt.Sprintf("snapshot error: %v", err))
		return
	}

	// Stage 2: Classify nodes into typed candidates
	walkCtx := &WalkContext{
		ScanID:       uuid.New().String(),
		LibraryID:    imp.libraryID,
		SourceRoot:   sourcePath,
		ClaimedPaths: make(map[string]PathClaim),
	}
	classResult := imp.classifyTree(ctx, rootNode, walkCtx)

	// Stage 3: Parse metadata by candidate type
	imp.parseCandidateMetadata(ctx, classResult.Candidates)

	// Set total_items to final candidate count
	total := len(classResult.Candidates)
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, 0, "running")

	// Stage 4: Reconcile candidates into DB writes
	reconResult, err := imp.reconcileCandidates(ctx, classResult.Candidates)
	if err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, fmt.Sprintf("reconcile error: %v", err))
		return
	}

	done := reconResult.DoneCandidates
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "done")
	_ = imp.jobRepo.UpdateSummary(ctx, jobID, imp.buildImportSummary(startTime, classResult, reconResult, nil))
}

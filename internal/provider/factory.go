package provider

import (
	"fmt"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/pkg/presign"
)

// FromRecord instantiates a Provider from a persisted ProviderRecord.
// Returns an error if the type is unknown or unsupported in this build.
//
// TODO(phase4): Add case "s3" once S3Provider is implemented.
// TODO(phase5): Add case "remote_fyom" once RemoteFyomProvider is implemented.
func FromRecord(rec model.ProviderRecord, signer *presign.Signer) (Provider, error) {
	switch rec.Type {
	case "local":
		// LocalProvider is always registered directly — it should never
		// appear in the providers table. This case is a safety guard.
		return NewLocalProvider(signer), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q for provider %q", rec.Type, rec.ID)
	}
}

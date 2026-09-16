package teacher

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNoAnswer is returned by an offline teacher for an input it has no record for.
var ErrNoAnswer = errors.New("teacher: no recorded answer for this input")

// OfflineJSONL answers from a record store and never reaches the network.
type OfflineJSONL struct {
	Store   *RecordStore
	ID      string
	Version string
}

// Ask looks the request's InputHash up; on a hit it returns the recorded
// response with the RequestID replaced by the incoming request's RequestID and
// everything else unchanged; on a miss it returns ErrNoAnswer. An invalid
// request is rejected before the lookup.
func (o OfflineJSONL) Ask(ctx context.Context, r Request) (Response, error) {
	if ctx == nil {
		return Response{}, fmt.Errorf("offline teacher: nil context")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if err := r.Validate(); err != nil {
		return Response{}, fmt.Errorf("offline teacher: %w", err)
	}
	if strings.TrimSpace(o.ID) == "" {
		return Response{}, fmt.Errorf("offline teacher: id must be non-blank")
	}
	if strings.TrimSpace(o.Version) == "" {
		return Response{}, fmt.Errorf("offline teacher: version must be non-blank")
	}
	if o.Store == nil {
		return Response{}, fmt.Errorf("offline teacher: nil store")
	}
	rec, ok := o.Store.Lookup(r.InputHash)
	if !ok {
		return Response{}, ErrNoAnswer
	}
	resp := rec.Response
	resp.RequestID = r.RequestID
	return resp, nil
}

// Describe returns {ID, Version, DescriptorOffline}.
func (o OfflineJSONL) Describe() Descriptor {
	return Descriptor{ID: o.ID, Version: o.Version, Kind: DescriptorOffline}
}

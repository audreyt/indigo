package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"

	"github.com/labstack/echo/v4"
	otel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("search")

func parseCursorLimit(e echo.Context) (int, int, error) {
	offset := 0
	if c := strings.TrimSpace(e.QueryParam("cursor")); c != "" {
		v, err := strconv.Atoi(c)
		if err != nil {
			return 0, 0, &echo.HTTPError{
				Code:    400,
				Message: fmt.Sprintf("invalid value for 'cursor': %s", err),
			}
		}
		offset = v
	}

	if offset < 0 {
		offset = 0
	}
	if offset > 10000 {
		return 0, 0, &echo.HTTPError{
			Code:    400,
			Message: fmt.Sprintf("invalid value for 'cursor' (can't paginate so deep)"),
		}
	}

	limit := 25
	if l := strings.TrimSpace(e.QueryParam("limit")); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil {
			return 0, 0, &echo.HTTPError{
				Code:    400,
				Message: fmt.Sprintf("invalid value for 'count': %s", err),
			}
		}

		limit = v
	}

	if limit > 100 {
		limit = 100
	}
	if limit < 0 {
		limit = 0
	}
	return offset, limit, nil
}

func (s *Server) handleSearchPostsSkeleton(e echo.Context) error {
	ctx, span := tracer.Start(e.Request().Context(), "handleSearchPostsSkeleton")
	defer span.End()

	span.SetAttributes(attribute.String("query", e.QueryParam("q")))

	q := strings.TrimSpace(e.QueryParam("q"))
	if q == "" {
		return e.JSON(400, map[string]any{
			"error": "must pass non-empty search query",
		})
	}

	offset, limit, err := parseCursorLimit(e)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("invalid cursor/limit: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("offset", offset), attribute.Int("limit", limit))

	out, err := s.SearchPosts(ctx, q, offset, limit)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("failed to SearchPosts: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("posts.length", len(out.Posts)))

	return e.JSON(200, out)
}

func (s *Server) handleStructuredSearchPostsSkeleton(e echo.Context) error {
	ctx, span := tracer.Start(e.Request().Context(), "handleStructuredSearchPostsSkeleton")
	defer span.End()

	query := SearchQuery{}

	span.SetAttributes(attribute.String("query", e.QueryParam("q")))

	q := strings.TrimSpace(e.QueryParam("q"))
	if q == "" {
		return e.JSON(400, map[string]any{
			"error": "must pass non-empty search query",
		})
	}
	query.Query = q

	offset, limit, err := parseCursorLimit(e)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("invalid cursor/limit: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("offset", offset), attribute.Int("limit", limit))

	query.Offset = offset
	query.Size = limit

	langs := e.Request().URL.Query()["langs"]
	if len(langs) > 0 {
		query.Langs = langs
	}

	actors := e.Request().URL.Query()["actors"]
	if len(actors) > 0 {
		query.Actors = actors
	}

	tags := e.Request().URL.Query()["tags"]
	if len(tags) > 0 {
		query.Tags = tags
	}

	from := e.Request().URL.Query().Get("from")
	if from != "" {
		// Parse as unix milliseconds
		asInt, err := strconv.ParseInt(from, 10, 64)
		if err != nil {
			span.SetAttributes(attribute.String("error", fmt.Sprintf("failed to parse 'from' timestamp: %s", err)))
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("failed to parse 'from' timestamp: %w", err)
		}
		t := time.Unix(0, asInt*int64(time.Millisecond))
		query.From = &t
	}

	to := e.Request().URL.Query().Get("to")
	if to != "" {
		// Parse as unix milliseconds
		asInt, err := strconv.ParseInt(to, 10, 64)
		if err != nil {
			span.SetAttributes(attribute.String("error", fmt.Sprintf("failed to parse 'to' timestamp: %s", err)))
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("failed to parse 'to' timestamp: %w", err)
		}
		t := time.Unix(0, asInt*int64(time.Millisecond))
		query.To = &t
	}

	out, err := s.StructuredSearchPosts(ctx, query)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("failed to SearchPosts: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("posts.length", len(out.Posts)))

	return e.JSON(200, out)
}

func (s *Server) handleSearchActorsSkeleton(e echo.Context) error {
	ctx, span := tracer.Start(e.Request().Context(), "handleSearchActorsSkeleton")
	defer span.End()

	span.SetAttributes(attribute.String("query", e.QueryParam("q")))

	q := strings.TrimSpace(e.QueryParam("q"))
	if q == "" {
		return e.JSON(400, map[string]any{
			"error": "must pass non-empty search query",
		})
	}

	offset, limit, err := parseCursorLimit(e)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("invalid cursor/limit: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	typeahead := false
	if q := strings.TrimSpace(e.QueryParam("typeahead")); q == "true" || q == "1" || q == "y" {
		typeahead = true
	}

	span.SetAttributes(
		attribute.Int("offset", offset),
		attribute.Int("limit", limit),
		attribute.Bool("typeahead", typeahead),
	)

	out, err := s.SearchProfiles(ctx, q, typeahead, offset, limit)
	if err != nil {
		span.SetAttributes(attribute.String("error", fmt.Sprintf("failed to SearchProfiles: %s", err)))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetAttributes(attribute.Int("actors.length", len(out.Actors)))

	return e.JSON(200, out)
}

func (s *Server) StructuredSearchPosts(ctx context.Context, q SearchQuery) (*appbsky.UnspeccedSearchPostsSkeleton_Output, error) {
	ctx, span := tracer.Start(ctx, "SearchPosts")
	defer span.End()

	span.SetAttributes(
		attribute.String("query", q.Query),
		attribute.Int("offset", q.Offset),
		attribute.Int("size", q.Size),
		attribute.StringSlice("actors", q.Actors),
		attribute.StringSlice("tags", q.Tags),
		attribute.StringSlice("langs", q.Langs),
	)

	resp, err := DoStructuredSearchPosts(ctx, s.dir, s.escli, s.postIndex, q)
	if err != nil {
		return nil, err
	}

	posts := []*appbsky.UnspeccedDefs_SkeletonSearchPost{}
	for _, r := range resp.Hits.Hits {
		var doc PostDoc
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			return nil, fmt.Errorf("decoding post doc from search response: %w", err)
		}

		did, err := syntax.ParseDID(doc.DID)
		if err != nil {
			return nil, fmt.Errorf("invalid DID in indexed document: %w", err)
		}

		posts = append(posts, &appbsky.UnspeccedDefs_SkeletonSearchPost{
			Uri: fmt.Sprintf("https://bsky.app/profile/%s/post/%s", did, doc.RecordRkey),
		})
	}

	out := appbsky.UnspeccedSearchPostsSkeleton_Output{Posts: posts}
	if len(posts) == q.Size && (q.Offset+q.Size) < 10000 {
		s := fmt.Sprintf("%d", q.Offset+q.Size)
		out.Cursor = &s
	}
	if resp.Hits.Total.Relation == "eq" {
		i := int64(resp.Hits.Total.Value)
		out.HitsTotal = &i
	}
	return &out, nil
}

func (s *Server) SearchPosts(ctx context.Context, q string, offset, size int) (*appbsky.UnspeccedSearchPostsSkeleton_Output, error) {
	ctx, span := tracer.Start(ctx, "SearchPosts")
	defer span.End()

	span.SetAttributes(
		attribute.String("query", q),
		attribute.Int("offset", offset),
		attribute.Int("size", size),
	)

	resp, err := DoSearchPosts(ctx, s.dir, s.escli, s.postIndex, q, offset, size)
	if err != nil {
		return nil, err
	}

	posts := []*appbsky.UnspeccedDefs_SkeletonSearchPost{}
	for _, r := range resp.Hits.Hits {
		var doc PostDoc
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			return nil, fmt.Errorf("decoding post doc from search response: %w", err)
		}

		did, err := syntax.ParseDID(doc.DID)
		if err != nil {
			return nil, fmt.Errorf("invalid DID in indexed document: %w", err)
		}

		posts = append(posts, &appbsky.UnspeccedDefs_SkeletonSearchPost{
			Uri: fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, doc.RecordRkey),
		})
	}

	out := appbsky.UnspeccedSearchPostsSkeleton_Output{Posts: posts}
	if len(posts) == size && (offset+size) < 10000 {
		s := fmt.Sprintf("%d", offset+size)
		out.Cursor = &s
	}
	if resp.Hits.Total.Relation == "eq" {
		i := int64(resp.Hits.Total.Value)
		out.HitsTotal = &i
	}
	return &out, nil
}

func (s *Server) SearchProfiles(ctx context.Context, q string, typeahead bool, offset, size int) (*appbsky.UnspeccedSearchActorsSkeleton_Output, error) {
	ctx, span := tracer.Start(ctx, "SearchProfiles")
	defer span.End()

	span.SetAttributes(
		attribute.String("query", q),
		attribute.Bool("typeahead", typeahead),
		attribute.Int("offset", offset),
		attribute.Int("size", size),
	)

	var resp *EsSearchResponse
	var err error
	if typeahead {
		resp, err = DoSearchProfilesTypeahead(ctx, s.escli, s.profileIndex, q, size)
	} else {
		resp, err = DoSearchProfiles(ctx, s.dir, s.escli, s.profileIndex, q, offset, size)
	}
	if err != nil {
		return nil, err
	}

	actors := []*appbsky.UnspeccedDefs_SkeletonSearchActor{}
	for _, r := range resp.Hits.Hits {
		var doc ProfileDoc
		if err := json.Unmarshal(r.Source, &doc); err != nil {
			return nil, fmt.Errorf("decoding profile doc from search response: %w", err)
		}

		did, err := syntax.ParseDID(doc.DID)
		if err != nil {
			return nil, fmt.Errorf("invalid DID in indexed document: %w", err)
		}

		actors = append(actors, &appbsky.UnspeccedDefs_SkeletonSearchActor{
			Did: did.String(),
		})
	}

	out := appbsky.UnspeccedSearchActorsSkeleton_Output{Actors: actors}
	if len(actors) == size && (offset+size) < 10000 {
		s := fmt.Sprintf("%d", offset+size)
		out.Cursor = &s
	}
	if resp.Hits.Total.Relation == "eq" {
		i := int64(resp.Hits.Total.Value)
		out.HitsTotal = &i
	}
	return &out, nil
}

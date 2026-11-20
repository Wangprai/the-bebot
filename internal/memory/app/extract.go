package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"the-bebot/internal/memory/core"
)

var ErrInvalidExtractionRequest = errors.New("invalid extraction request")

type ExtractionRequest struct {
	Snapshot     core.ConversationSnapshot
	TargetUserID string
}

type ExtractMemoriesUseCase struct {
	rules     core.CandidateRuleEvaluator
	extractor core.MemoryExtractor
	validator core.MemoryValidator
}

func NewExtractMemoriesUseCase(
	rules core.CandidateRuleEvaluator,
	extractor core.MemoryExtractor,
	validator core.MemoryValidator,
) *ExtractMemoriesUseCase {
	return &ExtractMemoriesUseCase{
		rules:     rules,
		extractor: extractor,
		validator: validator,
	}
}

func (uc *ExtractMemoriesUseCase) Execute(
	ctx context.Context,
	request ExtractionRequest,
) ([]core.Memory, error) {
	if uc == nil || uc.rules == nil ||
		uc.extractor == nil || uc.validator == nil {
		return nil, ErrInvalidExtractionRequest
	}

	if ctx == nil {
		return nil, ErrInvalidExtractionRequest
	}

	targetUserID := strings.TrimSpace(request.TargetUserID)
	if targetUserID == "" {
		return nil, ErrInvalidExtractionRequest
	}

	snapshot, err := core.NewConversationSnapshot(request.Snapshot)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			ErrInvalidExtractionRequest,
			err,
		)
	}

	shouldExtract, err := uc.rules.ShouldCreateCandidate(snapshot)
	if err != nil {
		return nil, fmt.Errorf("evaluate candidate rules: %w", err)
	}

	if !shouldExtract {
		return []core.Memory{}, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	extractionInput, err := core.NewExtractionInput(
		core.ExtractionInput{
			GuildID:   snapshot.GuildID,
			ChannelID: snapshot.ChannelID,
			UserID:    targetUserID,
			Messages:  snapshot.Messages,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("build extraction input: %w", err)
	}

	extractedMemories, err := uc.extractor.Extract(
		ctx,
		extractionInput,
	)
	if err != nil {
		return nil, fmt.Errorf("extract memories: %w", err)
	}

	memories := make([]core.Memory, 0, len(extractedMemories))

	for i, extracted := range extractedMemories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		source, err := findExtractionSource(
			snapshot,
			extracted,
			targetUserID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"validate extracted memory %d: %w",
				i,
				err,
			)
		}

		memory, err := uc.validator.Validate(extracted, source)
		if err != nil {
			return nil, fmt.Errorf(
				"validate extracted memory %d: %w",
				i,
				err,
			)
		}

		memories = append(memories, memory)
	}

	return memories, nil
}

func findExtractionSource(
	snapshot core.ConversationSnapshot,
	extracted core.ExtractedMemory,
	targetUserID string,
) (core.ExtractionSource, error) {
	sourceMessageID := strings.TrimSpace(extracted.SourceMessageID)

	if sourceMessageID == "" {
		return core.ExtractionSource{},
			core.ErrInvalidExtractionSource
	}

	for _, message := range snapshot.Messages {
		if message.MessageID != sourceMessageID {
			continue
		}

		if message.IsBot {
			return core.ExtractionSource{},
				core.ErrInvalidExtractionSource
		}

		if extracted.Scope == core.MemoryScopeMember &&
			message.UserID != targetUserID {
			return core.ExtractionSource{},
				core.ErrInvalidExtractionSource
		}

		return core.ExtractionSource{
			GuildID:   snapshot.GuildID,
			ChannelID: snapshot.ChannelID,
			MessageID: message.MessageID,
			UserID:    message.UserID,
		}, nil
	}

	return core.ExtractionSource{},
		core.ErrInvalidExtractionSource
}

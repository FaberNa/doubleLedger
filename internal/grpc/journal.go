package grpc

import (
	"context"
	"errors"
	"log"

	ledgerv1 "doubleLedger/api/gen/ledger/v1"
	"doubleLedger/internal/application"
	"doubleLedger/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// All stored amounts in the ledger are denominated in EUR cents.
const ledgerCurrencyCode = "EUR"

func (s *Server) PostJournalEntry(ctx context.Context, request *ledgerv1.PostJournalEntryRequest) (*ledgerv1.PostJournalEntryResponse, error) {
	inputs := make([]application.PostingInput, len(request.GetPostings()))
	for i, posting := range request.GetPostings() {
		inputs[i] = application.PostingInput{
			AccountID: posting.GetAccountId(),
			Side:      postingSide(posting.GetSide()),
			Amount:    domain.Amount(posting.GetAmountMinor()),
		}
	}
	entry, err := s.journal.Post(ctx, request.GetIdempotencyKey(), request.GetDescription(), inputs)
	switch {
	case err == nil:
		return &ledgerv1.PostJournalEntryResponse{JournalEntry: journalMessage(entry)}, nil
	case errors.Is(err, domain.ErrInvalidArgument), errors.Is(err, domain.ErrUnbalanced), errors.Is(err, domain.ErrOverflow):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, application.ErrAccountNotFound):
		return nil, status.Error(codes.NotFound, err.Error())
	case errors.Is(err, application.ErrIdempotencyConflict):
		return nil, status.Error(codes.AlreadyExists, err.Error())
	case ctx.Err() != nil:
		return nil, status.FromContextError(ctx.Err()).Err()
	default:
		log.Printf("post journal entry: %v", err)
		return nil, status.Error(codes.Internal, "could not post journal entry")
	}
}

func postingSide(side ledgerv1.Side) domain.Side {
	switch side {
	case ledgerv1.Side_SIDE_DEBIT:
		return domain.Debit
	case ledgerv1.Side_SIDE_CREDIT:
		return domain.Credit
	default:
		return ""
	}
}

func journalMessage(entry domain.JournalEntry) *ledgerv1.JournalEntry {
	postings := entry.Postings()
	lines := make([]*ledgerv1.Posting, len(postings))
	for i, p := range postings {
		side := ledgerv1.Side_SIDE_DEBIT
		if p.Side == domain.Credit {
			side = ledgerv1.Side_SIDE_CREDIT
		}
		lines[i] = &ledgerv1.Posting{
			Id:             p.ID,
			JournalEntryId: entry.ID(),
			AccountId:      p.AccountID,
			Side:           side,
			AmountMinor:    int64(p.Amount),
			LineNumber:     uint32(i + 1),
		}
	}
	return &ledgerv1.JournalEntry{
		Id:           entry.ID(),
		Description:  entry.Description(),
		PostedAt:     timestamppb.New(entry.PostedAt()),
		Postings:     lines,
		CurrencyCode: ledgerCurrencyCode,
	}
}

package grpc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ledgerv1 "doubleLedger/api/gen/ledger/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestPostJournalEntryPostgres(t *testing.T) {
	ctx, pool, client := postgresTestClient(t)
	accountIDs := make([]string, 3)
	for i, name := range []string{"Cash", "Service revenue", "Goods revenue"} {
		kind := ledgerv1.AccountType_ACCOUNT_TYPE_REVENUE
		if i == 0 {
			kind = ledgerv1.AccountType_ACCOUNT_TYPE_ASSET
		}
		response, err := client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{
			Code: testAccountCode(t, pool), Name: name, Type: kind,
		})
		if err != nil {
			t.Fatal(err)
		}
		accountIDs[i] = response.GetAccount().GetId()
	}

	newRequest := func(t *testing.T) *ledgerv1.PostJournalEntryRequest {
		t.Helper()
		key := uuid.NewString()
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := pool.Exec(ctx, `
				DELETE FROM ledger.postings WHERE journal_entry_id IN
				(SELECT id FROM ledger.journal_entries WHERE idempotency_key = $1)
			`, key); err != nil {
				t.Errorf("delete test postings: %v", err)
			}
			if _, err := pool.Exec(ctx, "DELETE FROM ledger.journal_entries WHERE idempotency_key = $1", key); err != nil {
				t.Errorf("delete test journal entry: %v", err)
			}
		})
		return &ledgerv1.PostJournalEntryRequest{
			IdempotencyKey: key,
			Description:    " Cash sale ",
			Postings: []*ledgerv1.PostingInput{
				{AccountId: accountIDs[0], Side: ledgerv1.Side_SIDE_DEBIT, AmountMinor: 10000},
				{AccountId: accountIDs[1], Side: ledgerv1.Side_SIDE_CREDIT, AmountMinor: 5000},
				{AccountId: accountIDs[2], Side: ledgerv1.Side_SIDE_CREDIT, AmountMinor: 5000},
			},
		}
	}

	t.Run("concurrent requests save one complete entry", func(t *testing.T) {
		request := newRequest(t)
		type result struct {
			response *ledgerv1.PostJournalEntryResponse
			err      error
		}
		results := make(chan result, 4)
		start := make(chan struct{})
		for range cap(results) {
			go func() {
				<-start
				response, err := client.PostJournalEntry(ctx, request)
				results <- result{response, err}
			}()
		}
		close(start)
		replies := make([]result, cap(results))
		for i := range replies {
			replies[i] = <-results
		}
		first := replies[0]
		if first.err != nil {
			t.Fatal(first.err)
		}
		entry := first.response.GetJournalEntry()
		for _, retry := range replies[1:] {
			if retry.err != nil || !proto.Equal(entry, retry.response.GetJournalEntry()) {
				t.Fatalf("retry returned %v, %v, want %v", retry.response, retry.err, entry)
			}
		}
		if entry.GetDescription() != "Cash sale" || entry.GetCurrencyCode() != "EUR" || len(entry.GetPostings()) != 3 {
			t.Fatalf("unexpected journal entry: %v", entry)
		}
		for i, posting := range entry.GetPostings() {
			input := request.GetPostings()[i]
			if posting.GetJournalEntryId() != entry.GetId() || posting.GetLineNumber() != uint32(i+1) ||
				posting.GetAccountId() != input.GetAccountId() || posting.GetSide() != input.GetSide() ||
				posting.GetAmountMinor() != input.GetAmountMinor() {
				t.Fatalf("unexpected posting %d: %v", i+1, posting)
			}
		}
		var entries, postings int
		err := pool.QueryRow(ctx, `
			SELECT count(DISTINCT e.id), count(p.id)
			FROM ledger.journal_entries AS e
			LEFT JOIN ledger.postings AS p ON p.journal_entry_id = e.id
			WHERE e.idempotency_key = $1
		`, request.GetIdempotencyKey()).Scan(&entries, &postings)
		if err != nil || entries != 1 || postings != 3 {
			t.Fatalf("stored %d entries and %d postings, query error %v", entries, postings, err)
		}

		normalized := proto.CloneOf(request)
		normalized.Description = "Cash sale"
		normalized.Postings[0].AccountId = strings.ToUpper(accountIDs[0])
		retry, err := client.PostJournalEntry(ctx, normalized)
		if err != nil || !proto.Equal(entry, retry.GetJournalEntry()) {
			t.Fatalf("normalized retry returned %v, %v", retry, err)
		}
		normalized.Postings[1], normalized.Postings[2] = normalized.Postings[2], normalized.Postings[1]
		if _, err := client.PostJournalEntry(ctx, normalized); status.Code(err) != codes.AlreadyExists {
			t.Fatalf("reordered retry returned %v, want AlreadyExists", err)
		}
	})

	t.Run("an unknown account rolls back the entire entry", func(t *testing.T) {
		request := newRequest(t)
		request.Postings[2].AccountId = uuid.NewString()
		if _, err := client.PostJournalEntry(ctx, request); status.Code(err) != codes.NotFound {
			t.Fatalf("PostJournalEntry returned %v, want NotFound", err)
		}
		var count int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM ledger.journal_entries WHERE idempotency_key = $1",
			request.GetIdempotencyKey()).Scan(&count)
		if err != nil || count != 0 {
			t.Fatalf("after rollback stored %d entries, query error %v", count, err)
		}
		request.Postings[2].AccountId = accountIDs[2]
		if _, err := client.PostJournalEntry(ctx, request); err != nil {
			t.Fatalf("could not reuse key after rollback: %v", err)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		request := newRequest(t)
		request.Postings[0].AmountMinor--
		if _, err := client.PostJournalEntry(ctx, request); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("unbalanced request returned %v, want InvalidArgument", err)
		}
		request.Postings[0].AmountMinor++
		request.IdempotencyKey = "contains a space"
		if _, err := client.PostJournalEntry(ctx, request); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("invalid key returned %v, want InvalidArgument", err)
		}
	})
}

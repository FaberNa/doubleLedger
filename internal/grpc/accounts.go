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

type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	accounts *application.Accounts
}

func NewServer(accounts *application.Accounts) *Server {
	return &Server{accounts: accounts}
}

func (s *Server) CreateAccount(ctx context.Context, request *ledgerv1.CreateAccountRequest) (*ledgerv1.CreateAccountResponse, error) {
	account, err := s.accounts.Create(ctx, request.GetCode(), request.GetName(), accountType(request.GetType()))
	switch {
	case err == nil:
		return &ledgerv1.CreateAccountResponse{Account: &ledgerv1.Account{
			Id:        account.ID(),
			Code:      account.Code(),
			Name:      account.Name(),
			Type:      request.GetType(),
			CreatedAt: timestamppb.New(account.CreatedAt()),
		}}, nil
	case errors.Is(err, domain.ErrInvalidArgument):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, application.ErrAccountCodeExists):
		return nil, status.Error(codes.AlreadyExists, err.Error())
	case ctx.Err() != nil:
		return nil, status.FromContextError(ctx.Err()).Err()
	default:
		log.Printf("create account: %v", err)
		return nil, status.Error(codes.Internal, "could not create account")
	}
}

func accountType(value ledgerv1.AccountType) domain.AccountType {
	switch value {
	case ledgerv1.AccountType_ACCOUNT_TYPE_ASSET:
		return domain.Asset
	case ledgerv1.AccountType_ACCOUNT_TYPE_LIABILITY:
		return domain.Liability
	case ledgerv1.AccountType_ACCOUNT_TYPE_EQUITY:
		return domain.Equity
	case ledgerv1.AccountType_ACCOUNT_TYPE_EXPENSE:
		return domain.Expense
	case ledgerv1.AccountType_ACCOUNT_TYPE_REVENUE:
		return domain.Revenue
	default:
		return ""
	}
}

package ledger

type EntryType string

const (
	Deposit  EntryType = "DEPOSIT"
	Withdraw EntryType = "WITHDRAW"
)

type Entry struct {
	ID      int64
	UserID  int64
	Amount  int64
	Type    EntryType
	OrderID *int64
}
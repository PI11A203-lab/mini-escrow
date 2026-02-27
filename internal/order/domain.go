package order

import "errors"

type Status string

const (
	Created   Status = "CREATED"
	Funded    Status = "FUNDED"
	Confirmed Status = "CONFIRMED"
	Cancelled Status = "CANCELLED"
)

type Order struct {
	ID       int64
	BuyerID  int64
	SellerID int64
	Amount   int64
	Status   Status
	Version  int64
}

func (o *Order) Fund() error {
	if o.Status != Created {
		return errors.New("invalid state transition")
	}
	o.Status = Funded
	return nil
}

func (o *Order) Confirm() error {
	if o.Status != Funded {
		return errors.New("only FUNDED order can be confirmed")
	}
	o.Status = Confirmed
	return nil
}

func (o *Order) Cancel() error {
	if o.Status != Funded {
		return errors.New("only FUNDED order can be cancelled")
	}
	o.Status = Cancelled
	return nil
}

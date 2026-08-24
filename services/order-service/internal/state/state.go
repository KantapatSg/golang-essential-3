package state

import "errors"

const (
	Pending       = "PENDING"
	StockReserved = "STOCK_RESERVED"
	Cancelling    = "CANCELLING"
	Confirmed     = "CONFIRMED"
	Rejected      = "REJECTED"
	Cancelled     = "CANCELLED"
)

// Transition เป็นจุดเดียวที่ยอมให้ consumer เปลี่ยนสถานะ จึงกัน event ซ้ำและ event มาถึงผิดลำดับ
func Transition(current, event string) (string, error) {
	switch current {
	case Pending:
		switch event {
		case "InventoryReserved":
			return StockReserved, nil
		case "InventoryRejected":
			return Rejected, nil
		case "PaymentCompleted":
			return "", errors.New("payment before reservation")
		case "PaymentFailed":
			return "", errors.New("payment before reservation")
		}
	case StockReserved:
		switch event {
		case "PaymentCompleted":
			return Confirmed, nil
		case "PaymentFailed":
			return Cancelling, nil
		case "InventoryRejected":
			return "", errors.New("stock rejection after reservation")
		}
	case Cancelling:
		if event == "InventoryReleased" {
			return Cancelled, nil
		}
	case Confirmed, Rejected, Cancelled:
		return current, nil
	}
	return "", errors.New("illegal order transition")
}

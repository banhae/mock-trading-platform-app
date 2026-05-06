package main

import "errors"

var ErrInvalidIncomingOrder = errors.New("invalid incoming order")

type IncomingOrder struct {
	OrderID           string
	Side              string
	Price             int64
	RemainingQuantity int64
}

type Fill struct {
	TradeID      string
	MakerOrderID string
	TakerOrderID string
	Price        int64 // maker price
	Quantity     int64
}

type MatchResult struct {
	TakerOrderID           string
	TakerSide              string
	TakerLimitPrice        int64
	TakerRemainingQuantity int64
	TakerRested            bool
	Fills                  []Fill
}

type CancelStatus string

const (
	CancelStatusCancelled CancelStatus = "cancelled"
	CancelStatusNotInBook CancelStatus = "not_in_book"
)

type CancelResult struct {
	OrderID           string
	Status            CancelStatus
	Side              string
	Price             int64
	RemainingQuantity int64
}

func ProcessCancelOrder(book *OrderBook, orderID string) (CancelResult, error) {
	if book == nil || orderID == "" {
		return CancelResult{}, ErrOrderNotInBook
	}

	removed, ok := book.RemoveOrderByID(orderID)
	if !ok {
		return CancelResult{OrderID: orderID, Status: CancelStatusNotInBook}, nil
	}

	return CancelResult{
		OrderID:           removed.OrderID,
		Status:            CancelStatusCancelled,
		Side:              removed.Side,
		Price:             removed.Price,
		RemainingQuantity: removed.RemainingQuantity,
	}, nil
}

func ProcessNewOrder(book *OrderBook, incoming IncomingOrder) (MatchResult, error) {
	if book == nil {
		return MatchResult{}, ErrInvalidIncomingOrder
	}
	if incoming.OrderID == "" || incoming.Price <= 0 || incoming.RemainingQuantity <= 0 {
		return MatchResult{}, ErrInvalidIncomingOrder
	}
	if incoming.Side != "buy" && incoming.Side != "sell" {
		return MatchResult{}, ErrInvalidIncomingOrder
	}
	if _, exists := book.byID[incoming.OrderID]; exists {
		return MatchResult{}, ErrDuplicateOrderID
	}

	result := MatchResult{
		TakerOrderID:           incoming.OrderID,
		TakerSide:              incoming.Side,
		TakerLimitPrice:        incoming.Price,
		TakerRemainingQuantity: incoming.RemainingQuantity,
		Fills:                  make([]Fill, 0),
	}

	for result.TakerRemainingQuantity > 0 {
		makerNode := bestOppositeMakerNode(book, incoming.Side)
		if makerNode == nil {
			break
		}
		if !isCrossed(incoming.Side, incoming.Price, makerNode.order.Price) {
			break
		}

		fillQty := minInt64(result.TakerRemainingQuantity, makerNode.order.RemainingQuantity)
		makerRemaining, err := SafeSubInt64(makerNode.order.RemainingQuantity, fillQty)
		if err != nil {
			return MatchResult{}, err
		}
		takerRemaining, err := SafeSubInt64(result.TakerRemainingQuantity, fillQty)
		if err != nil {
			return MatchResult{}, err
		}

		makerNode.order.RemainingQuantity = makerRemaining
		result.TakerRemainingQuantity = takerRemaining
		result.Fills = append(result.Fills, Fill{
			MakerOrderID: makerNode.order.OrderID,
			TakerOrderID: incoming.OrderID,
			Price:        makerNode.order.Price,
			Quantity:     fillQty,
		})

		if makerRemaining == 0 {
			if makerNode.order.Side == "buy" {
				book.bids.removeNode(makerNode)
			} else {
				book.asks.removeNode(makerNode)
			}
			delete(book.byID, makerNode.order.OrderID)
		}
	}

	if result.TakerRemainingQuantity > 0 {
		_, err := book.AppendRestingOrder(BookOrder{
			OrderID:           incoming.OrderID,
			Side:              incoming.Side,
			Price:             incoming.Price,
			RemainingQuantity: result.TakerRemainingQuantity,
		})
		if err != nil {
			return MatchResult{}, err
		}
		result.TakerRested = true
	}

	return result, nil
}

func bestOppositeMakerNode(book *OrderBook, takerSide string) *bookNode {
	if takerSide == "buy" {
		bestAsk := book.asks.bestLevel()
		if bestAsk == nil {
			return nil
		}
		return bestAsk.head
	}
	bestBid := book.bids.bestLevel()
	if bestBid == nil {
		return nil
	}
	return bestBid.head
}

func isCrossed(takerSide string, takerPrice, makerPrice int64) bool {
	if takerSide == "buy" {
		return makerPrice <= takerPrice
	}
	return makerPrice >= takerPrice
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

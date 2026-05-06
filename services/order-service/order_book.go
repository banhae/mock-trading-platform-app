package main

import "errors"

var (
	ErrInvalidBookSide   = errors.New("invalid book side")
	ErrDuplicateOrderID  = errors.New("duplicate order id")
	ErrOrderNotInBook    = errors.New("order not found in book")
	ErrInvalidBookValues = errors.New("invalid book price or quantity")
)

// BookOrder is the in-memory resting-order shape used by matcher core.
// 가격/수량은 Slice A 규칙대로 scaled integer(int64)를 사용한다.
type BookOrder struct {
	OrderID           string
	Side              string
	Price             int64
	RemainingQuantity int64
	Sequence          uint64 // deterministic FIFO assertion을 위한 내부 순번
}

type bookNode struct {
	order BookOrder
	prev  *bookNode
	next  *bookNode
	level *PriceLevel
}

type PriceLevel struct {
	Price int64
	head  *bookNode
	tail  *bookNode
	len   int
}

func (l *PriceLevel) append(node *bookNode) {
	node.prev = l.tail
	node.next = nil
	node.level = l
	if l.tail != nil {
		l.tail.next = node
	} else {
		l.head = node
	}
	l.tail = node
	l.len++
}

func (l *PriceLevel) remove(node *bookNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}
	node.prev = nil
	node.next = nil
	node.level = nil
	l.len--
}

func (l *PriceLevel) headOrder() (BookOrder, bool) {
	if l == nil || l.head == nil {
		return BookOrder{}, false
	}
	return l.head.order, true
}

type BookSide struct {
	isBid  bool
	levels map[int64]*PriceLevel
	prices []int64 // bid: desc, ask: asc
}

func newBookSide(isBid bool) *BookSide {
	return &BookSide{
		isBid:  isBid,
		levels: make(map[int64]*PriceLevel),
		prices: make([]int64, 0),
	}
}

func (s *BookSide) addNode(node *bookNode) {
	level, ok := s.levels[node.order.Price]
	if !ok {
		level = &PriceLevel{Price: node.order.Price}
		s.levels[node.order.Price] = level
		s.insertPrice(node.order.Price)
	}
	level.append(node)
}

func (s *BookSide) bestLevel() *PriceLevel {
	if len(s.prices) == 0 {
		return nil
	}
	return s.levels[s.prices[0]]
}

func (s *BookSide) removeNode(node *bookNode) {
	level := node.level
	if level == nil {
		return
	}
	level.remove(node)
	if level.len == 0 {
		delete(s.levels, level.Price)
		s.removePrice(level.Price)
	}
}

func (s *BookSide) insertPrice(price int64) {
	idx := 0
	for idx < len(s.prices) {
		p := s.prices[idx]
		if s.isBid {
			if price > p {
				break
			}
		} else {
			if price < p {
				break
			}
		}
		idx++
	}
	s.prices = append(s.prices, 0)
	copy(s.prices[idx+1:], s.prices[idx:])
	s.prices[idx] = price
}

func (s *BookSide) removePrice(price int64) {
	for i, p := range s.prices {
		if p == price {
			s.prices = append(s.prices[:i], s.prices[i+1:]...)
			return
		}
	}
}

// OrderBook is a single-pair in-memory order book.
type OrderBook struct {
	bids    *BookSide
	asks    *BookSide
	byID    map[string]*bookNode
	nextSeq uint64
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids: newBookSide(true),
		asks: newBookSide(false),
		byID: make(map[string]*bookNode),
	}
}

func (b *OrderBook) AppendRestingOrder(order BookOrder) (BookOrder, error) {
	if order.OrderID == "" || order.Price <= 0 || order.RemainingQuantity <= 0 {
		return BookOrder{}, ErrInvalidBookValues
	}
	if _, exists := b.byID[order.OrderID]; exists {
		return BookOrder{}, ErrDuplicateOrderID
	}
	if order.Side != "buy" && order.Side != "sell" {
		return BookOrder{}, ErrInvalidBookSide
	}

	b.nextSeq++
	order.Sequence = b.nextSeq

	node := &bookNode{order: order}
	if order.Side == "buy" {
		b.bids.addNode(node)
	} else {
		b.asks.addNode(node)
	}
	b.byID[order.OrderID] = node
	return order, nil
}

func (b *OrderBook) BestBid() (BookOrder, bool) {
	return b.bids.bestLevel().headOrder()
}

func (b *OrderBook) BestAsk() (BookOrder, bool) {
	return b.asks.bestLevel().headOrder()
}

func (b *OrderBook) RemoveOrderByID(orderID string) (BookOrder, bool) {
	node, ok := b.byID[orderID]
	if !ok {
		return BookOrder{}, false
	}
	if node.order.Side == "buy" {
		b.bids.removeNode(node)
	} else {
		b.asks.removeNode(node)
	}
	delete(b.byID, orderID)
	return node.order, true
}

// SideSnapshot returns resting orders in strict matching traversal order.
func (b *OrderBook) SideSnapshot(side string) []BookOrder {
	var selected *BookSide
	if side == "buy" {
		selected = b.bids
	} else if side == "sell" {
		selected = b.asks
	} else {
		return []BookOrder{}
	}

	out := make([]BookOrder, 0)
	for _, price := range selected.prices {
		level := selected.levels[price]
		for cur := level.head; cur != nil; cur = cur.next {
			out = append(out, cur.order)
		}
	}
	return out
}

func (b *OrderBook) HasPriceLevel(side string, price int64) bool {
	if side == "buy" {
		_, ok := b.bids.levels[price]
		return ok
	} else if side == "sell" {
		_, ok := b.asks.levels[price]
		return ok
	}
	return false
}

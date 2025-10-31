package model

import "github.com/google/uuid"

type OrderCreated struct {
	OrderID    uuid.UUID
	CustomerID uuid.UUID
}

func (e OrderCreated) Type() string {
	return "OrderCreated"
}

type OrderItemChanged struct {
	OrderID      uuid.UUID
	AddedItems   []uuid.UUID
	RemovedItems []uuid.UUID
}

func (e OrderItemChanged) Type() string {
	return "OrderItemChanged"
}

type OrderStatusChanged struct {
	OrderID   uuid.UUID
	OldStatus OrderStatus
	NewStatus OrderStatus
}

func (e OrderStatusChanged) Type() string {
	return "OrderStatusChanged"
}

type OrderDeleted struct {
	OrderID uuid.UUID
}

func (e OrderDeleted) Type() string {
	return "OrderDeleted"
}

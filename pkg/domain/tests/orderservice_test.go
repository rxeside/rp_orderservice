package tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"order/pkg/domain/model"
	"order/pkg/domain/service"
)

func setup(t *testing.T) (service.Order, *mockOrderRepository, *mockEventDispatcher) {
	repo := &mockOrderRepository{
		store: make(map[uuid.UUID]*model.Order),
	}
	eventDispatcher := &mockEventDispatcher{}
	orderService := service.NewOrderService(repo, eventDispatcher)
	return orderService, repo, eventDispatcher
}

func TestOrderService_CreateOrder(t *testing.T) {
	orderService, repo, dispatcher := setup(t)
	customerID := uuid.New()

	orderID, err := orderService.CreateOrder(customerID)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, orderID)

	savedOrder, ok := repo.store[orderID]
	require.True(t, ok)
	assert.Equal(t, customerID, savedOrder.CustomerID)
	assert.Equal(t, model.Open, savedOrder.Status)

	require.Len(t, dispatcher.events, 1)
	createdEvent, ok := dispatcher.events[0].(model.OrderCreated)
	require.True(t, ok)
	assert.Equal(t, orderID, createdEvent.OrderID)
	assert.Equal(t, customerID, createdEvent.CustomerID)
}

func TestOrderService_AddItem(t *testing.T) {
	orderService, repo, dispatcher := setup(t)
	customerID := uuid.New()
	orderID, _ := orderService.CreateOrder(customerID)
	dispatcher.Reset()

	productID := uuid.New()
	price := 100.50

	t.Run("Success on open order", func(t *testing.T) {
		itemID, err := orderService.AddItem(orderID, productID, price)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, itemID)

		order, _ := repo.Find(orderID)
		require.Len(t, order.Items, 1)
		assert.Equal(t, itemID, order.Items[0].ID)

		require.Len(t, dispatcher.events, 1)
		event, ok := dispatcher.events[0].(model.OrderItemChanged)
		require.True(t, ok)
		assert.Equal(t, []uuid.UUID{itemID}, event.AddedItems)
	})

	t.Run("Fail on pending order", func(t *testing.T) {
		repo.store[orderID].Status = model.Pending
		dispatcher.Reset()

		_, err := orderService.AddItem(orderID, uuid.New(), 200)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrInvalidOrderStatus)
		assert.Empty(t, dispatcher.events)
	})
}

func TestOrderService_DeleteItem(t *testing.T) {
	orderService, repo, dispatcher := setup(t)
	customerID := uuid.New()
	orderID, _ := orderService.CreateOrder(customerID)
	itemID, _ := orderService.AddItem(orderID, uuid.New(), 150)
	dispatcher.Reset()

	t.Run("Success on open order", func(t *testing.T) {
		err := orderService.DeleteItem(orderID, itemID)
		require.NoError(t, err)

		order, _ := repo.Find(orderID)
		assert.Empty(t, order.Items)

		require.Len(t, dispatcher.events, 1)
		event, ok := dispatcher.events[0].(model.OrderItemChanged)
		require.True(t, ok)
		assert.Equal(t, []uuid.UUID{itemID}, event.RemovedItems)
	})

	t.Run("Fail on non-existent item", func(t *testing.T) {
		dispatcher.Reset()
		err := orderService.DeleteItem(orderID, uuid.New())
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrOrderItemNotFound)
		assert.Empty(t, dispatcher.events)
	})
}

func TestOrderService_SetStatus(t *testing.T) {
	orderService, repo, dispatcher := setup(t)
	orderID, _ := orderService.CreateOrder(uuid.New())

	testCases := []struct {
		name          string
		fromStatus    model.OrderStatus
		toStatus      model.OrderStatus
		expectSuccess bool
	}{
		{"Open to Pending", model.Open, model.Pending, true},
		{"Open to Cancelled", model.Open, model.Cancelled, true},
		{"Pending to Paid", model.Pending, model.Paid, true},
		{"Pending to Cancelled", model.Pending, model.Cancelled, true},
		{"Open to Paid (Invalid)", model.Open, model.Paid, false},
		{"Paid to Cancelled (Invalid)", model.Paid, model.Cancelled, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := repo.store[orderID]
			order.Status = tc.fromStatus
			repo.store[orderID] = order
			dispatcher.Reset()

			err := orderService.SetStatus(orderID, tc.toStatus)

			if tc.expectSuccess {
				require.NoError(t, err)
				assert.Equal(t, tc.toStatus, repo.store[orderID].Status)
				require.Len(t, dispatcher.events, 1)
				event, ok := dispatcher.events[0].(model.OrderStatusChanged)
				require.True(t, ok)
				assert.Equal(t, tc.fromStatus, event.OldStatus)
				assert.Equal(t, tc.toStatus, event.NewStatus)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, service.ErrInvalidStatusTransition)
				assert.Empty(t, dispatcher.events)
				assert.Equal(t, tc.fromStatus, repo.store[orderID].Status)
			}
		})
	}
}

func TestOrderService_DeleteOrder(t *testing.T) {
	orderService, repo, dispatcher := setup(t)
	orderID, _ := orderService.CreateOrder(uuid.New())

	dispatcher.Reset()

	err := orderService.DeleteOrder(orderID)
	require.NoError(t, err)

	order := repo.store[orderID]
	require.NotNil(t, order.DeletedAt)

	_, err = repo.Find(orderID)
	assert.ErrorIs(t, err, model.ErrOrderNotFound)

	require.Len(t, dispatcher.events, 1)
	_, ok := dispatcher.events[0].(model.OrderDeleted)
	require.True(t, ok)
}

// --- Mocks ---

var _ model.OrderRepository = &mockOrderRepository{}

type mockOrderRepository struct {
	store map[uuid.UUID]*model.Order
}

func (m *mockOrderRepository) NextID() (uuid.UUID, error) {
	return uuid.NewRandom()
}

func (m *mockOrderRepository) Store(order *model.Order) error {
	m.store[order.ID] = order
	return nil
}

func (m *mockOrderRepository) Find(id uuid.UUID) (*model.Order, error) {
	if order, ok := m.store[id]; ok && order.DeletedAt == nil {
		return order, nil
	}
	return nil, model.ErrOrderNotFound
}

func (m *mockOrderRepository) Delete(id uuid.UUID) error {
	if order, ok := m.store[id]; ok && order.DeletedAt == nil {
		now := time.Now().UTC()
		order.DeletedAt = &now
		m.store[id] = order
		return nil
	}
	return model.ErrOrderNotFound
}

var _ service.EventDispatcher = &mockEventDispatcher{}

type mockEventDispatcher struct {
	events []service.Event
}

func (m *mockEventDispatcher) Dispatch(event service.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventDispatcher) Reset() {
	m.events = nil
}

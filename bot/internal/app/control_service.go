package app

import (
	"context"

	"bot/internal/batchstate"
	"bot/internal/erp"
	"core/batchcontrol"
	"core/workflow"
)

func (a *App) newControlService() *batchcontrol.Service {
	return batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    controlCatalog{client: a.erp},
		BatchState: controlBatchStateWriter{store: a.batchState},
		Runner:     a.newMaterialReceiptRunner(),
		Logger:     a.logBatch,
	})
}

type controlCatalog struct {
	client *erp.Client
}

func (c controlCatalog) CheckConnection(ctx context.Context) (string, error) {
	return c.client.CheckConnection(ctx)
}

func (c controlCatalog) SearchItems(ctx context.Context, query string, limit int) ([]batchcontrol.Item, error) {
	items, err := c.client.SearchItems(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]batchcontrol.Item, 0, len(items))
	for _, item := range items {
		out = append(out, batchcontrol.Item{
			Name:     item.Name,
			ItemCode: item.ItemCode,
			ItemName: item.ItemName,
		})
	}
	return out, nil
}

func (c controlCatalog) SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]batchcontrol.WarehouseStock, error) {
	stocks, err := c.client.SearchItemWarehouses(ctx, itemCode, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]batchcontrol.WarehouseStock, 0, len(stocks))
	for _, stock := range stocks {
		out = append(out, batchcontrol.WarehouseStock{
			Warehouse: stock.Warehouse,
			ActualQty: stock.ActualQty,
		})
	}
	return out, nil
}

type controlBatchStateWriter struct {
	store *batchstate.Store
}

func (w controlBatchStateWriter) Set(active bool, ownerID int64, selection workflow.Selection) error {
	if w.store == nil {
		return nil
	}
	selection = selection.Normalize()
	return w.store.Set(active, ownerID, selection.ItemCode, selection.ItemName, selection.Warehouse)
}

import { describe, expect, it, vi } from "vitest";
import { createStore } from "jotai";

vi.mock("src/api", () => ({
  GetPurchaseOrders: vi.fn().mockResolvedValue([]),
  GetNextPurchaseOrderNumber: vi.fn(),
  GetPurchaseOrder: vi.fn(),
  GetPurchaseOrderLineItems: vi.fn().mockResolvedValue([]),
  CreatePurchaseOrder: vi.fn(),
  UpdatePurchaseOrder: vi.fn(),
  UpdatePurchaseOrderStatus: vi.fn(),
  DeletePurchaseOrder: vi.fn(),
  GetOrders: vi.fn().mockResolvedValue([]),
  GetNextOrderNumber: vi.fn(),
  GetOrder: vi.fn(),
  GetOrderLineItems: vi.fn().mockResolvedValue([]),
  CreateOrder: vi.fn(),
  UpdateOrder: vi.fn(),
  UpdateOrderStatus: vi.fn(),
  DeleteOrder: vi.fn(),
  GetDeliveries: vi.fn().mockResolvedValue([]),
  GetNextDeliveryNumber: vi.fn(),
  GetDelivery: vi.fn(),
  GetDeliveryLineItems: vi.fn().mockResolvedValue([]),
  CreateDelivery: vi.fn(),
  UpdateDelivery: vi.fn(),
  UpdateDeliveryStatus: vi.fn(),
  DeleteDelivery: vi.fn(),
  GetInboundDeliveries: vi.fn().mockResolvedValue([]),
  GetNextInboundDeliveryNumber: vi.fn(),
  GetInboundDelivery: vi.fn(),
  GetInboundDeliveryLineItems: vi.fn().mockResolvedValue([]),
  CreateInboundDelivery: vi.fn(),
  UpdateInboundDelivery: vi.fn(),
  UpdateInboundDeliveryStatus: vi.fn(),
  DeleteInboundDelivery: vi.fn(),
  GetProductionOrders: vi.fn().mockResolvedValue([]),
  GetNextProductionOrderNumber: vi.fn(),
  GetProductionOrder: vi.fn(),
  GetProductionOrderComponentLines: vi.fn().mockResolvedValue([]),
  CreateProductionOrder: vi.fn(),
  UpdateProductionOrderStatus: vi.fn(),
  DeleteProductionOrder: vi.fn(),
}));

vi.mock("src/utils/message", () => ({
  message: { success: vi.fn(), error: vi.fn() },
}));

import {
  GetNextPurchaseOrderNumber,
  CreatePurchaseOrder,
  GetNextOrderNumber,
  CreateOrder,
  GetNextDeliveryNumber,
  CreateDelivery,
  GetNextInboundDeliveryNumber,
  CreateInboundDelivery,
  GetNextProductionOrderNumber,
  CreateProductionOrder,
} from "src/api";
import type {
  PurchaseOrder,
  Order,
  Delivery,
  InboundDelivery,
  ProductionOrder,
} from "src/types/models";
import { organizationIdAtom } from "src/atoms/organization";
import {
  nextPurchaseOrderNumberAtom,
  purchaseOrderIdAtom,
  purchaseOrderAtom,
} from "./purchase-order";
import { nextOrderNumberAtom, orderIdAtom, orderAtom } from "./order";
import { nextDeliveryNumberAtom, deliveryIdAtom, deliveryAtom } from "./delivery";
import {
  nextInboundDeliveryNumberAtom,
  inboundDeliveryIdAtom,
  inboundDeliveryAtom,
} from "./inbound-delivery";
import {
  nextProductionOrderNumberAtom,
  productionOrderIdAtom,
  createProductionOrderAtom,
} from "./production-order";

// Each next-number atom is refreshable via atomWithRefresh. The write atom's
// create branch calls set(nextXAtom) immediately after setting the new id,
// which invalidates the cached value so the next read re-fetches from the
// server. This test exercises the purchase-order path end-to-end; the other
// four atoms use the identical pattern.

describe("next-number atoms refresh after create", () => {
  it("purchase-order: refreshes after creating a document", async () => {
    const store = createStore();
    store.set(organizationIdAtom, "org-1");
    store.set(purchaseOrderIdAtom, null);

    vi.mocked(GetNextPurchaseOrderNumber)
      .mockResolvedValueOnce("PO-0001")
      .mockResolvedValueOnce("PO-0002");
    vi.mocked(CreatePurchaseOrder).mockResolvedValue({
      id: "po-new",
      organizationId: "org-1",
    } as unknown as PurchaseOrder);

    const num1 = await store.get(nextPurchaseOrderNumberAtom);
    expect(num1).toBe("PO-0001");
    expect(GetNextPurchaseOrderNumber).toHaveBeenCalledTimes(1);

    await store.set(purchaseOrderAtom, { vendorId: "v-1", orderDate: Date.now() });

    const num2 = await store.get(nextPurchaseOrderNumberAtom);
    expect(num2).toBe("PO-0002");
    expect(GetNextPurchaseOrderNumber).toHaveBeenCalledTimes(2);
  });

  it("order: refreshes after creating a document", async () => {
    const store = createStore();
    store.set(organizationIdAtom, "org-1");
    store.set(orderIdAtom, null);

    vi.mocked(GetNextOrderNumber).mockResolvedValueOnce("ORD-001").mockResolvedValueOnce("ORD-002");
    vi.mocked(CreateOrder).mockResolvedValue({
      id: "order-new",
      organizationId: "org-1",
    } as unknown as Order);

    const num1 = await store.get(nextOrderNumberAtom);
    expect(num1).toBe("ORD-001");
    expect(GetNextOrderNumber).toHaveBeenCalledTimes(1);

    await store.set(orderAtom, { clientId: "c-1", orderDate: Date.now() });

    const num2 = await store.get(nextOrderNumberAtom);
    expect(num2).toBe("ORD-002");
    expect(GetNextOrderNumber).toHaveBeenCalledTimes(2);
  });

  it("delivery: refreshes after creating a document", async () => {
    const store = createStore();
    store.set(organizationIdAtom, "org-1");
    store.set(deliveryIdAtom, null);

    vi.mocked(GetNextDeliveryNumber)
      .mockResolvedValueOnce("DEL-0001")
      .mockResolvedValueOnce("DEL-0002");
    vi.mocked(CreateDelivery).mockResolvedValue({
      id: "del-new",
      organizationId: "org-1",
    } as unknown as Delivery);

    const num1 = await store.get(nextDeliveryNumberAtom);
    expect(num1).toBe("DEL-0001");
    expect(GetNextDeliveryNumber).toHaveBeenCalledTimes(1);

    await store.set(deliveryAtom, { clientId: "c-1", deliveryDate: Date.now() });

    const num2 = await store.get(nextDeliveryNumberAtom);
    expect(num2).toBe("DEL-0002");
    expect(GetNextDeliveryNumber).toHaveBeenCalledTimes(2);
  });

  it("inbound-delivery: refreshes after creating a document", async () => {
    const store = createStore();
    store.set(organizationIdAtom, "org-1");
    store.set(inboundDeliveryIdAtom, null);

    vi.mocked(GetNextInboundDeliveryNumber)
      .mockResolvedValueOnce("GR-0001")
      .mockResolvedValueOnce("GR-0002");
    vi.mocked(CreateInboundDelivery).mockResolvedValue({
      id: "gr-new",
      organizationId: "org-1",
    } as unknown as InboundDelivery);

    const num1 = await store.get(nextInboundDeliveryNumberAtom);
    expect(num1).toBe("GR-0001");
    expect(GetNextInboundDeliveryNumber).toHaveBeenCalledTimes(1);

    await store.set(inboundDeliveryAtom, {
      vendorId: "v-1",
      deliveryDate: Date.now(),
    });

    const num2 = await store.get(nextInboundDeliveryNumberAtom);
    expect(num2).toBe("GR-0002");
    expect(GetNextInboundDeliveryNumber).toHaveBeenCalledTimes(2);
  });

  it("production-order: refreshes after creating a document", async () => {
    const store = createStore();
    store.set(organizationIdAtom, "org-1");
    store.set(productionOrderIdAtom, null);

    vi.mocked(GetNextProductionOrderNumber)
      .mockResolvedValueOnce("PRO-0001")
      .mockResolvedValueOnce("PRO-0002");
    vi.mocked(CreateProductionOrder).mockResolvedValue({
      id: "pro-new",
      organizationId: "org-1",
    } as unknown as ProductionOrder);

    const num1 = await store.get(nextProductionOrderNumberAtom);
    expect(num1).toBe("PRO-0001");
    expect(GetNextProductionOrderNumber).toHaveBeenCalledTimes(1);

    await store.set(createProductionOrderAtom, { finishedProductId: "p-1" });

    const num2 = await store.get(nextProductionOrderNumberAtom);
    expect(num2).toBe("PRO-0002");
    expect(GetNextProductionOrderNumber).toHaveBeenCalledTimes(2);
  });
});

export interface Item {
  id: string;
  name: string;
}

export interface Order {
  item_type_id: string;
  city: string;
  quality_level: number;
  unit_price_silver: number;
  amount: number;
  auction_type: string;
  captured_at: string;
  weekly_avg?: number | null;
}

export interface MarketPrice {
  City: string;
  Quality: number;
  MinPrice: number;
  AvgPrice: number;
  Supply: number;
  NumOrders: number;
  PriceClass: string;
}

export async function searchItems(q: string): Promise<Item[]> {
  const res = await fetch(`/api/search?q=${encodeURIComponent(q)}`);
  if (!res.ok) return [];
  const data = await res.json();
  return data || [];
}

export async function fetchRecentOrders(limit = 50): Promise<Order[]> {
  const res = await fetch(`/api/orders/recent?limit=${limit}`);
  if (!res.ok) throw new Error(`Failed to fetch orders: ${res.status}`);
  const data = await res.json();
  return data || [];
}

export interface MarketHistoryPoint {
  timestamp: number;
  city: string;
  per_item: number;
  timescale: number;
}

export async function fetchItemPrices(id: string): Promise<MarketPrice[]> {
  const res = await fetch(`/api/item?id=${encodeURIComponent(id)}`);
  if (!res.ok) throw new Error(`Failed to fetch prices: ${res.status}`);
  const data = await res.json();
  return data || [];
}

export async function fetchItemHistory(id: string, timescale = 0): Promise<MarketHistoryPoint[]> {
  const res = await fetch(`/api/item/history?id=${encodeURIComponent(id)}&timescale=${timescale}`);
  if (!res.ok) throw new Error(`Failed to fetch history: ${res.status}`);
  const data = await res.json();
  return data || [];
}

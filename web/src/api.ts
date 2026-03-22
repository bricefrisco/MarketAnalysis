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
  monthly_avg?: number | null;
  profit?: number | null;
  profit_pct?: number | null;
}

export interface OrdersResponse {
  orders: Order[];
  total: number;
  page: number;
  page_size: number;
}

export async function searchItems(q: string): Promise<Item[]> {
  const res = await fetch(`/api/search?q=${encodeURIComponent(q)}`);
  if (!res.ok) return [];
  const data = await res.json();
  return data || [];
}

export async function fetchRecentOrders(page = 1, pageSize = 25): Promise<OrdersResponse> {
  const res = await fetch(`/api/orders/recent?page=${page}&page_size=${pageSize}`);
  if (!res.ok) throw new Error(`Failed to fetch orders: ${res.status}`);
  return res.json();
}

export interface CraftingResource {
  item_type_id: string;
  name: string;
  count: number;
  no_return: boolean;
  avg_price: number;
}

export interface CraftingItem {
  item_type_id: string;
  name: string;
  tier: number;
  category: string;
  sub_category: string;
  resources: CraftingResource[];
  avg_sell_price: number;
}

export interface CraftingResponse {
  items: CraftingItem[];
  location_id: string;
}

export async function fetchCrafting(city: string): Promise<CraftingResponse> {
  const res = await fetch(`/api/crafting?city=${encodeURIComponent(city)}`);
  if (!res.ok) throw new Error(`Failed to fetch crafting data: ${res.status}`);
  return res.json();
}

export async function clearData(): Promise<{ status: string }> {
  const res = await fetch('/api/clear-data', { method: 'POST' });
  if (!res.ok) throw new Error(`Failed to clear data: ${res.status}`);
  return res.json();
}


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
  quality: number;
  category: string;
  sub_category: string;
  resources: CraftingResource[];
  avg_sell_price: number;
  current_sell_price: number;
  crafting_focus: number;
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

export async function clearCraftingPrices(): Promise<void> {
  const res = await fetch('/api/crafting/prices', { method: 'DELETE' });
  if (!res.ok) throw new Error(`Failed to clear prices: ${res.status}`);
}

export interface RefiningRow {
  tier: number;
  resource_type: string;
  bonus_city_id: string;
  bonus_city_name: string;
  raw_city_id: string;
  raw_city_name: string;
  raw_t2_buy_price: number;
  raw_t3_buy_price: number;
  raw_t4_buy_price: number;
  raw_t5_buy_price: number; // only populated for tier=5
  refined_sell_price: number;
  raw_cost: number;
  refined_item_value: number;
  profit_per_item: number;
  batches_per_trip: number;
  profit_per_trip: number;
}

export interface RefiningResponse {
  rows: RefiningRow[];
}

export async function fetchRefining(): Promise<RefiningResponse> {
  const res = await fetch('/api/refining');
  if (!res.ok) throw new Error(`Failed to fetch refining data: ${res.status}`);
  return res.json();
}

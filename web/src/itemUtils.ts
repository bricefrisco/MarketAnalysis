export interface ItemMeta {
  tier: number;
  enchantment: number;
  /** e.g. "4.4" — omits enchantment suffix when it's 0 */
  label: string;
}

/** Parse tier and enchantment from an Albion item type ID.
 *  e.g. "T4_MAIN_HOLYSTAFF_AVALON@4" → { tier: 4, enchantment: 4, label: "4.4" }
 *       "T6_BAG"                      → { tier: 6, enchantment: 0, label: "6"   }
 */
export function parseItemMeta(id: string): ItemMeta {
  const tierMatch = id.match(/^T(\d+)/i);
  const enchantMatch = id.match(/@(\d+)$/);
  const tier = tierMatch ? parseInt(tierMatch[1], 10) : 0;
  const enchantment = enchantMatch ? parseInt(enchantMatch[1], 10) : 0;
  const label = enchantment > 0 ? `${tier}.${enchantment}` : `${tier}`;
  return { tier, enchantment, label };
}

const QUALITY_LABELS: Record<number, string> = {
  1: 'Normal',
  2: 'Good',
  3: 'Outstanding',
  4: 'Excellent',
  5: 'Masterpiece',
};

const QUALITY_COLORS: Record<number, string> = {
  1: '#9e9e9e',
  2: '#4ade80',
  3: '#60a5fa',
  4: '#c084fc',
  5: '#f59e0b',
};

export function qualityLabel(quality: number): string {
  return QUALITY_LABELS[quality] ?? `Q${quality}`;
}

export function qualityColor(quality: number): string {
  return QUALITY_COLORS[quality] ?? '#9e9e9e';
}

const TIER_COLORS: Record<number, string> = {
  1: '#9e9e9e',
  2: '#8bc34a',
  3: '#26c6da',
  4: '#42a5f5',
  5: '#ab47bc',
  6: '#ef5350',
  7: '#ff9800',
  8: '#ffd600',
};

export function tierColor(tier: number): string {
  return TIER_COLORS[tier] ?? '#9e9e9e';
}

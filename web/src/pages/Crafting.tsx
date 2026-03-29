import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Box, Container, Typography, Paper, TextField,
  Table, TableHead, TableBody, TableRow, TableCell,
  Skeleton, Alert, Chip, ToggleButtonGroup, ToggleButton, Tooltip,
  InputAdornment, Button,
} from '@mui/material';
import BuildIcon from '@mui/icons-material/Build';
import SearchIcon from '@mui/icons-material/Search';
import { type CraftingItem, fetchCrafting, clearCraftingPrices } from '../api';
import { tierColor, parseItemMeta } from '../itemUtils';

const PAGE_SIZE = 50;

// ── Constants ────────────────────────────────────────────────────────────

const CITIES = [
  { id: '0007', name: 'Thetford' },
  { id: '1002', name: 'Lymhurst' },
  { id: '2004', name: 'Bridgewatch' },
  { id: '3008', name: 'Martlock' },
  { id: '4002', name: 'Fort Sterling' },
];

const MARKET_TAX = 0.065;
const RRR_BASE        = 15.25; // no focus, no bonus city
const RRR_BONUS_CITY  = 24.81; // bonus city, no focus
const RRR_FOCUS       = 43.50; // focus, no bonus city
const RRR_FOCUS_BONUS = 47.92; // focus + bonus city

const QUALITY_LABELS: Record<number, string> = {
  1: 'Normal',
  2: 'Good',
  3: 'Outstanding',
  4: 'Excellent',
};

// sub_category values → display label
const SUB_CATEGORY_LABELS: Record<string, string> = {
  sword: 'Sword', axe: 'Axe', bow: 'Bow', crossbow: 'Crossbow',
  dagger: 'Dagger', firestaff: 'Fire Staff', froststaff: 'Frost Staff',
  holystaff: 'Holy Staff', arcanestaff: 'Arcane Staff', naturestaff: 'Nature Staff',
  cursestaff: 'Curse Staff', quarterstaff: 'Quarterstaff', hammer: 'Hammer',
  mace: 'Mace', spear: 'Spear', knuckles: 'Knuckles',
  cloth_armor: 'Cloth Chest', leather_armor: 'Leather Chest', plate_armor: 'Plate Chest',
  cloth_helmet: 'Cloth Helmet', leather_helmet: 'Leather Helmet', plate_helmet: 'Plate Helmet',
  cloth_shoes: 'Cloth Shoes', leather_shoes: 'Leather Shoes', plate_shoes: 'Plate Shoes',
  shieldtype: 'Shield', booktype: 'Book', torchtype: 'Torch',
};

// ── Profit calculation ────────────────────────────────────────────────────────

interface CalcResult {
  resourceCost: number;
  effectiveCost: number;
  profit: number | null;
  profitPct: number | null;
}

function calcResult(item: CraftingItem, rrr: number, usageFee: number): CalcResult | null {
  let resourceCost = 0;
  let returnableValue = 0;

  for (const res of item.resources) {
    if (res.avg_price <= 0) return null; // can't compute cost without all resource prices
    const cost = res.count * res.avg_price;
    resourceCost += cost;
    if (!res.no_return) returnableValue += cost;
  }

  if (resourceCost <= 0) return null;

  const effectiveCost = resourceCost - returnableValue * (rrr / 100);

  const sellPrice = item.current_sell_price > 0 ? item.current_sell_price : item.avg_sell_price;
  if (sellPrice <= 0) {
    return { resourceCost, effectiveCost, profit: null, profitPct: null };
  }

  // Usage fee: (crafting_focus / 100) * fee_value  (silver per 100 nutrition)
  const fee = item.crafting_focus > 0 ? (item.crafting_focus / 100) * usageFee : 0;
  const revenue = sellPrice * (1 - MARKET_TAX);
  const profit = revenue - effectiveCost - fee;
  const profitPct = (profit / effectiveCost) * 100;

  return { resourceCost, effectiveCost, profit, profitPct };
}

// ── Component ─────────────────────────────────────────────────────────────

export function Crafting() {
  const [items, setItems] = useState<CraftingItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [liveIndicator, setLiveIndicator] = useState(false);
  const [clearing, setClearing] = useState(false);

  // Controls
  const [city, setCity] = useState('2004');
  const [useFocus, setUseFocus] = useState(false);
  const [bonusCity, setBonusCity] = useState(false);
  const [rrr, setRrr] = useState(RRR_BASE);
  const [usageFee, setUsageFee] = useState(300);

  // Filters
  const [category, setCategory] = useState<string>('all');
  const [tier, setTier] = useState<number>(4);
  const [enchantment, setEnchantment] = useState<number | null>(null); // null = all
  const [search, setSearch] = useState('');

  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const cityRef = useRef(city);
  cityRef.current = city;

  // Initial fetch when city changes
  useEffect(() => {
    setLoading(true);
    setError(null);
    fetchCrafting(city)
      .then((data) => setItems(data.items))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [city]);

  // WebSocket for live price updates
  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/crafting`);
    wsRef.current = ws;

    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data) as { location_id: string };
        if (data.location_id !== cityRef.current) return;

        // Debounce: wait 2 s after last event before re-fetching
        if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current);
        refreshTimeoutRef.current = setTimeout(() => {
          fetchCrafting(cityRef.current)
            .then((d) => {
              setItems(d.items);
              setLiveIndicator(true);
              setTimeout(() => setLiveIndicator(false), 2000);
            })
            .catch(() => {/* silent */});
        }, 2000);
      } catch {/* ignore parse errors */}
    };

    return () => {
      ws.close();
      if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current);
    };
  }, []);

  const handleFocus = (on: boolean) => {
    setUseFocus(on);
    setRrr(on
      ? (bonusCity ? RRR_FOCUS_BONUS : RRR_FOCUS)
      : (bonusCity ? RRR_BONUS_CITY  : RRR_BASE));
  };

  const handleBonusCity = (on: boolean) => {
    setBonusCity(on);
    setRrr(on
      ? (useFocus ? RRR_FOCUS_BONUS : RRR_BONUS_CITY)
      : (useFocus ? RRR_FOCUS       : RRR_BASE));
  };

  const handleRrr = (val: string) => {
    setRrr(parseFloat(val) || 0);
  };

  const handleClearPrices = () => {
    setClearing(true);
    clearCraftingPrices()
      .then(() => fetchCrafting(city))
      .then((data) => setItems(data.items))
      .catch((err) => setError(err.message))
      .finally(() => setClearing(false));
  };

  const filterItem = (item: CraftingItem, q: string) => {
    const meta = parseItemMeta(item.item_type_id);
    if (item.tier !== tier) return false;
    if (enchantment !== null && meta.enchantment !== enchantment) return false;
    if (category !== 'all' && item.category !== category) return false;
    if (q && !item.name.toLowerCase().includes(q) && !item.item_type_id.toLowerCase().includes(q)) return false;
    return true;
  };

  // Filter + compute profit + sort + limit
  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    return items
      .filter((item) => filterItem(item, q))
      .map((item) => ({ item, result: calcResult(item, rrr, usageFee) }))
      .sort((a, b) => {
        const pa = a.result?.profitPct ?? null;
        const pb = b.result?.profitPct ?? null;
        if (pa === null && pb === null) return 0;
        if (pa === null) return 1;
        if (pb === null) return -1;
        return pb - pa;
      })
      .slice(0, PAGE_SIZE);
  }, [items, tier, enchantment, category, rrr, usageFee, search]);

  const fmt = (n: number) => Math.round(n).toLocaleString();

  const totalFiltered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return items.filter((item) => filterItem(item, q)).length;
  }, [items, tier, enchantment, category, search]);

  const fmtPctOrDash = (n: number | null) => n !== null ? `${n > 0 ? '+' : ''}${n.toFixed(1)}%` : '—';

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 3 }}>
        <BuildIcon sx={{ color: 'primary.main' }} />
        <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em">
          Crafting Calculator
        </Typography>
        {liveIndicator && (
          <Chip label="Updated" size="small" color="success" sx={{ height: 20, fontSize: '0.7rem' }} />
        )}
      </Box>

      {/* Controls */}
      <Paper elevation={0} sx={{ p: 2.5, mb: 2.5 }}>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, alignItems: 'flex-end' }}>
          {/* City */}
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              City
            </Typography>
            <ToggleButtonGroup
              value={city}
              exclusive
              size="small"
              onChange={(_, v) => v && setCity(v)}
            >
              {CITIES.map((c) => (
                <ToggleButton key={c.id} value={c.id} sx={{ px: 1.5, fontSize: '0.75rem' }}>
                  {c.name}
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          </Box>

          {/* RRR */}
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Resource Return Rate
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <TextField
                size="small"
                value={rrr}
                onChange={(e) => handleRrr(e.target.value)}
                sx={{ width: 90 }}
                InputProps={{ endAdornment: <InputAdornment position="end">%</InputAdornment> }}
              />
              <Button
                size="small"
                variant={useFocus ? 'contained' : 'outlined'}
                onClick={() => handleFocus(!useFocus)}
                sx={{ whiteSpace: 'nowrap', fontSize: '0.72rem' }}
              >
                Focus
              </Button>
              <Button
                size="small"
                variant={bonusCity ? 'contained' : 'outlined'}
                onClick={() => handleBonusCity(!bonusCity)}
                sx={{ whiteSpace: 'nowrap', fontSize: '0.72rem' }}
              >
                Bonus City
              </Button>
            </Box>
          </Box>

          {/* Usage fee */}
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Usage Fee <Typography component="span" variant="caption" color="text.disabled">(1–999)</Typography>
            </Typography>
            <TextField
              size="small"
              value={usageFee}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                if (!isNaN(v)) setUsageFee(Math.min(999, Math.max(1, v)));
              }}
              sx={{ width: 90 }}
              inputProps={{ min: 1, max: 999, type: 'number' }}
            />
          </Box>

          {/* Tier filter */}
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Tier
            </Typography>
            <ToggleButtonGroup
              value={tier}
              exclusive
              size="small"
              onChange={(_, v) => v !== null && setTier(v)}
            >
              {[4, 5, 6, 7, 8].map((t) => (
                <ToggleButton
                  key={t}
                  value={t}
                  sx={{
                    px: 1.5,
                    fontSize: '0.75rem',
                    fontWeight: 700,
                    color: tierColor(t),
                    '&.Mui-selected': { color: tierColor(t), backgroundColor: `${tierColor(t)}22` },
                  }}
                >
                  T{t}
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          </Box>

          {/* Enchantment filter */}
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Enchantment
            </Typography>
            <ToggleButtonGroup
              value={enchantment ?? -1}
              exclusive
              size="small"
              onChange={(_, v) => v !== null && setEnchantment(v === -1 ? null : v)}
            >
              <ToggleButton value={-1} sx={{ px: 1.5, fontSize: '0.75rem' }}>
                All
              </ToggleButton>
              {[0, 1, 2, 3, 4].map((e) => (
                <ToggleButton key={e} value={e} sx={{ px: 1.5, fontSize: '0.75rem', fontWeight: 600 }}>
                  .{e}
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          </Box>
        </Box>

        {/* Category filter + search */}
        <Box sx={{ mt: 2, display: 'flex', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
          <ToggleButtonGroup
            value={category}
            exclusive
            size="small"
            onChange={(_, v) => v && setCategory(v)}
          >
            {['all', 'weapon', 'armor', 'offhand'].map((cat) => (
              <ToggleButton key={cat} value={cat} sx={{ px: 2, fontSize: '0.75rem', textTransform: 'capitalize' }}>
                {cat === 'all' ? 'All' : cat === 'offhand' ? 'Off-hand' : cat.charAt(0).toUpperCase() + cat.slice(1)}
              </ToggleButton>
            ))}
          </ToggleButtonGroup>
          <TextField
            size="small"
            placeholder="Search items…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            sx={{ width: 220 }}
            InputProps={{ startAdornment: <InputAdornment position="start"><SearchIcon sx={{ fontSize: 18, color: 'text.disabled' }} /></InputAdornment> }}
          />
          <Box sx={{ ml: 'auto' }}>
            <Button
              size="small"
              variant="outlined"
              color="warning"
              disabled={clearing || loading}
              onClick={handleClearPrices}
            >
              {clearing ? 'Clearing…' : 'Clear Prices'}
            </Button>
          </Box>
        </Box>
      </Paper>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      {!loading && (
        <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
          Showing {Math.min(PAGE_SIZE, totalFiltered)} of {totalFiltered} rows
          {totalFiltered > PAGE_SIZE ? ' — refine your search to see more' : ''}
        </Typography>
      )}

      {/* Table */}
      <Paper elevation={0}>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Item</TableCell>
              <TableCell>Quality</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Resources</TableCell>
              <TableCell align="right">Resource Cost</TableCell>
              <TableCell align="right">Effective Cost</TableCell>
              <TableCell align="right">Sell Avg (4w)</TableCell>
              <TableCell align="right">Lowest Ask</TableCell>
              <TableCell align="right">Profit</TableCell>
              <TableCell align="right">Profit %</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 15 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 10 }).map((_, j) => (
                      <TableCell key={j}><Skeleton variant="text" /></TableCell>
                    ))}
                  </TableRow>
                ))
              : rows.map(({ item, result }, idx) => {
                  const noData = !result;
                  return (
                    <TableRow
                      key={idx}
                      sx={{ opacity: noData ? 0.4 : 1 }}
                    >
                      {/* Item name + tier */}
                      <TableCell>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography
                            variant="caption"
                            fontWeight={700}
                            sx={{ color: tierColor(item.tier), minWidth: 24, whiteSpace: 'nowrap' }}
                          >
                            T{parseItemMeta(item.item_type_id).label}
                          </Typography>
                          <Typography variant="body2" fontWeight={500} color="text.primary"
                            sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 220 }}>
                            {item.name || item.item_type_id}
                          </Typography>
                        </Box>
                      </TableCell>

                      {/* Quality */}
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {QUALITY_LABELS[item.quality] ?? `Q${item.quality}`}
                        </Typography>
                      </TableCell>

                      {/* Type */}
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {SUB_CATEGORY_LABELS[item.sub_category] ?? item.sub_category}
                        </Typography>
                      </TableCell>

                      {/* Resources */}
                      <TableCell>
                        <Tooltip
                          title={
                            <Box>
                              {item.resources.map((r, i) => (
                                <Typography key={i} variant="caption" display="block">
                                  {r.count}× {r.name || r.item_type_id}
                                  {r.no_return ? ' (artefact)' : ''}
                                  {r.avg_price > 0 ? ` — ${fmt(r.avg_price)} ea` : ' — no data'}
                                </Typography>
                              ))}
                            </Box>
                          }
                          placement="right"
                        >
                          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, cursor: 'default' }}>
                            {item.resources.map((r, i) => (
                              <Chip
                                key={i}
                                label={`${r.count}× ${r.name?.split("'s ").pop()?.split(' ').slice(0, 2).join(' ') ?? r.item_type_id}`}
                                size="small"
                                variant={r.no_return ? 'filled' : 'outlined'}
                                sx={{
                                  height: 18,
                                  fontSize: '0.65rem',
                                  opacity: r.avg_price > 0 ? 1 : 0.5,
                                  backgroundColor: r.no_return ? 'rgba(251,191,36,0.12)' : undefined,
                                  borderColor: r.no_return ? 'rgba(251,191,36,0.4)' : undefined,
                                }}
                              />
                            ))}
                          </Box>
                        </Tooltip>
                      </TableCell>

                      {/* Resource Cost */}
                      <TableCell align="right">
                        <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                          {result ? fmt(result.resourceCost) : '—'}
                        </Typography>
                      </TableCell>

                      {/* Effective Cost (after RRR) */}
                      <TableCell align="right">
                        <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                          {result ? fmt(result.effectiveCost) : '—'}
                        </Typography>
                      </TableCell>

                      {/* Sell Avg */}
                      <TableCell align="right">
                        <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                          {item.avg_sell_price > 0 ? fmt(item.avg_sell_price) : '—'}
                        </Typography>
                      </TableCell>

                      {/* Lowest Ask */}
                      <TableCell align="right">
                        <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                          {item.current_sell_price > 0 ? fmt(item.current_sell_price) : '—'}
                        </Typography>
                      </TableCell>

                      {/* Profit */}
                      <TableCell align="right">
                        <Typography
                          variant="body2"
                          fontWeight={600}
                          color={result?.profit != null ? (result.profit > 0 ? 'success.main' : 'error.main') : 'text.disabled'}
                          sx={{ fontVariantNumeric: 'tabular-nums' }}
                        >
                          {result?.profit != null ? `${result.profit > 0 ? '+' : ''}${fmt(result.profit)}` : '—'}
                        </Typography>
                      </TableCell>

                      {/* Profit % */}
                      <TableCell align="right">
                        <Typography
                          variant="body2"
                          fontWeight={700}
                          color={result?.profitPct != null ? (result.profitPct > 0 ? 'success.main' : 'error.main') : 'text.disabled'}
                          sx={{ fontVariantNumeric: 'tabular-nums' }}
                        >
                          {fmtPctOrDash(result?.profitPct ?? null)}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  );
                })}
          </TableBody>
        </Table>
      </Paper>
    </Container>
  );
}

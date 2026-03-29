import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Box, Container, Typography, Paper, Chip,
  Table, TableHead, TableBody, TableRow, TableCell,
  Skeleton, Alert, ToggleButtonGroup, ToggleButton, TextField,
} from '@mui/material';
import PrecisionManufacturingIcon from '@mui/icons-material/PrecisionManufacturing';
import { type RefiningRow, fetchRefining } from '../api';
import { tierColor } from '../itemUtils';

// ── Constants ─────────────────────────────────────────────────────────────

const RESOURCE_TYPES = ['Fiber', 'Ore', 'Wood', 'Hide', 'Rock'];

const KEEP = 1 - 0.367; // city bonus return rate

// Net raw consumed per T4 output in steady-state chain refining.
//   T4 raw: keep × 2        = 1.266
//   T3 raw: keep² × 2       = 0.801
//   T2 raw: keep³ × 1       = 0.254
const NET_T4_T4CHAIN = KEEP * 2;
const NET_T3_T4CHAIN = KEEP * KEEP * 2;
const NET_T2_T4CHAIN = KEEP * KEEP * KEEP * 1;

// Net raw consumed per T5 output in steady-state chain refining.
//   T5 raw: keep × 3        = 1.899
//   T4 raw: keep² × 2       = 0.801
//   T3 raw: keep³ × 2       = 0.507
//   T2 raw: keep⁴ × 1       = 0.161
const NET_T5_T5CHAIN = KEEP * 3;
const NET_T4_T5CHAIN = KEEP * KEEP * 2;
const NET_T3_T5CHAIN = KEEP * KEEP * KEEP * 2;
const NET_T2_T5CHAIN = KEEP * KEEP * KEEP * KEEP * 1;

const T4_BATCH_WEIGHT_KG = NET_T4_T4CHAIN * 0.51 + NET_T3_T4CHAIN * 0.34 + NET_T2_T4CHAIN * 0.23;
const T5_BATCH_WEIGHT_KG = NET_T5_T5CHAIN * 0.82 + NET_T4_T5CHAIN * 0.51 + NET_T3_T5CHAIN * 0.34 + NET_T2_T5CHAIN * 0.23;

const INVENTORY_SLOTS = 48;
const STACK_SIZE = 999;

// Maximum batches constrained by inventory slots for T4 chain (3 raw types).
function calcSlotLimitedBatches(slots: number, stackSize: number): number {
  let lo = 0, hi = slots * stackSize;
  while (lo < hi) {
    const mid = Math.floor((lo + hi + 1) / 2);
    const slotsNeeded = Math.ceil(mid * NET_T4_T4CHAIN / stackSize)
                      + Math.ceil(mid * NET_T3_T4CHAIN / stackSize)
                      + Math.ceil(mid * NET_T2_T4CHAIN / stackSize);
    if (slotsNeeded <= slots) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

// Maximum batches constrained by inventory slots for T5 chain (4 raw types).
function calcSlotLimitedBatchesT5(slots: number, stackSize: number): number {
  let lo = 0, hi = slots * stackSize;
  while (lo < hi) {
    const mid = Math.floor((lo + hi + 1) / 2);
    const slotsNeeded = Math.ceil(mid * NET_T5_T5CHAIN / stackSize)
                      + Math.ceil(mid * NET_T4_T5CHAIN / stackSize)
                      + Math.ceil(mid * NET_T3_T5CHAIN / stackSize)
                      + Math.ceil(mid * NET_T2_T5CHAIN / stackSize);
    if (slotsNeeded <= slots) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

// ── Helpers ───────────────────────────────────────────────────────────────

function fmtSilver(n: number): string {
  if (n === 0) return '—';
  if (Math.abs(n) >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (Math.abs(n) >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return Math.round(n).toLocaleString();
}

function fmtPrice(n: number): string {
  return n > 0 ? Math.round(n).toLocaleString() : '—';
}

// ── Component ──────────────────────────────────────────────────────────────

export function Refining() {
  const [rows, setRows] = useState<RefiningRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [liveIndicator, setLiveIndicator] = useState(false);
  const [resourceFilter, setResourceFilter] = useState<string>('all');
  const [tierFilter, setTierFilter] = useState<4 | 5>(4);
  const [stationFee, setStationFee] = useState(0);
  const [carryWeight, setCarryWeight] = useState(25735);

  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);
    fetchRefining()
      .then((data) => setRows(data.rows))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  // WebSocket for live price updates
  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/refining`);

    ws.onmessage = () => {
      if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = setTimeout(() => {
        fetchRefining()
          .then((d) => {
            setRows(d.rows);
            setLiveIndicator(true);
            setTimeout(() => setLiveIndicator(false), 2000);
          })
          .catch(() => {/* silent */});
      }, 2000);
    };

    return () => {
      ws.close();
      if (refreshTimeoutRef.current) clearTimeout(refreshTimeoutRef.current);
    };
  }, []);

  // Returns rows grouped by Buy From city, profit-sorted within each city group.
  const groupedRows = useMemo(() => {
    const batchWeightKg = tierFilter === 4 ? T4_BATCH_WEIGHT_KG : T5_BATCH_WEIGHT_KG;
    const weightBatches = Math.floor(carryWeight / batchWeightKg);
    const slotBatches = tierFilter === 4
      ? calcSlotLimitedBatches(INVENTORY_SLOTS, STACK_SIZE)
      : calcSlotLimitedBatchesT5(INVENTORY_SLOTS, STACK_SIZE);
    const batches = Math.min(weightBatches, slotBatches);
    const limitedBy: 'weight' | 'slots' = weightBatches <= slotBatches ? 'weight' : 'slots';

    const r = rows
      .filter((row) => row.tier === tierFilter)
      .filter((row) => resourceFilter === 'all' || row.resource_type === resourceFilter);

    const adjusted = r.map((row) => {
      // Usage Fee = ((ItemValue × 0.1125) × StationFee) / 100, paid per refining step in the chain.
      // T4 chain: steps T2(4) + T3(8) + T4(16)
      // T5 chain: steps T2(4) + T3(8) + T4(16) + T5(32)
      const chainItemValue = tierFilter === 4
        ? (4 + 8 + row.refined_item_value)
        : (4 + 8 + 16 + row.refined_item_value);
      const usageFee = ((chainItemValue * 0.1125) * stationFee) / 100;
      const profitPerItem = row.profit_per_item - usageFee;
      return { ...row, profit_per_item: profitPerItem, batches_per_trip: batches, profit_per_trip: profitPerItem * batches };
    });

    // Group by raw_city_name, preserving city order from the server.
    const cityOrder: string[] = [];
    const byCity = new Map<string, typeof adjusted>();
    for (const row of adjusted) {
      if (!byCity.has(row.raw_city_name)) {
        cityOrder.push(row.raw_city_name);
        byCity.set(row.raw_city_name, []);
      }
      byCity.get(row.raw_city_name)!.push(row);
    }

    // Sort each city's rows by profit/trip descending.
    for (const city of cityOrder) {
      byCity.get(city)!.sort((a, b) => b.profit_per_trip - a.profit_per_trip);
    }

    if (tierFilter === 4) {
      return {
        cityOrder, byCity, batches, limitedBy,
        t5Count: 0,
        t4Count: Math.ceil(batches * NET_T4_T4CHAIN),
        t3Count: Math.ceil(batches * NET_T3_T4CHAIN),
        t2Count: Math.ceil(batches * NET_T2_T4CHAIN),
      };
    }
    return {
      cityOrder, byCity, batches, limitedBy,
      t5Count: Math.ceil(batches * NET_T5_T5CHAIN),
      t4Count: Math.ceil(batches * NET_T4_T5CHAIN),
      t3Count: Math.ceil(batches * NET_T3_T5CHAIN),
      t2Count: Math.ceil(batches * NET_T2_T5CHAIN),
    };
  }, [rows, resourceFilter, tierFilter, stationFee, carryWeight]);

  const colCount = tierFilter === 4 ? 11 : 12;

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 3 }}>
        <PrecisionManufacturingIcon sx={{ color: 'primary.main' }} />
        <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em">
          Refining Calculator
        </Typography>
        {liveIndicator && (
          <Chip label="Updated" size="small" color="success" sx={{ height: 20, fontSize: '0.7rem' }} />
        )}
      </Box>

      {/* Controls */}
      <Paper elevation={0} sx={{ p: 2.5, mb: 2.5 }}>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, alignItems: 'flex-end' }}>
          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Tier
            </Typography>
            <ToggleButtonGroup
              value={tierFilter}
              exclusive
              size="small"
              onChange={(_, v) => v && setTierFilter(v as 4 | 5)}
            >
              <ToggleButton value={4} sx={{ px: 1.5, fontSize: '0.75rem' }}>T4</ToggleButton>
              <ToggleButton value={5} sx={{ px: 1.5, fontSize: '0.75rem' }}>T5</ToggleButton>
            </ToggleButtonGroup>
          </Box>

          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Resource
            </Typography>
            <ToggleButtonGroup
              value={resourceFilter}
              exclusive
              size="small"
              onChange={(_, v) => v && setResourceFilter(v)}
            >
              <ToggleButton value="all" sx={{ px: 1.5, fontSize: '0.75rem' }}>
                All
              </ToggleButton>
              {RESOURCE_TYPES.map((rt) => (
                <ToggleButton key={rt} value={rt} sx={{ px: 1.5, fontSize: '0.75rem' }}>
                  {rt}
                </ToggleButton>
              ))}
            </ToggleButtonGroup>
          </Box>

          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Station Fee <Typography component="span" variant="caption" color="text.disabled">(0–999)</Typography>
            </Typography>
            <TextField
              size="small"
              value={stationFee}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                setStationFee(isNaN(v) ? 0 : Math.min(999, Math.max(0, v)));
              }}
              sx={{ width: 120 }}
              inputProps={{ min: 0, max: 999, type: 'number' }}
            />
          </Box>

          <Box>
            <Typography variant="caption" color="text.secondary" fontWeight={600} sx={{ mb: 0.75, display: 'block' }}>
              Carry Weight (kg)
            </Typography>
            <TextField
              size="small"
              value={carryWeight}
              onChange={(e) => {
                const v = parseInt(e.target.value, 10);
                setCarryWeight(isNaN(v) ? 0 : Math.max(0, v));
              }}
              sx={{ width: 140 }}
              inputProps={{ min: 0, type: 'number' }}
            />
          </Box>

          <Box sx={{ ml: 'auto', alignSelf: 'flex-end' }}>
            <Typography variant="caption" color="text.secondary">
              T{tierFilter} chain refining · city bonus · no focus
            </Typography>
          </Box>
        </Box>
      </Paper>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      {!loading && (
        <Paper elevation={0} sx={{ p: 2, mb: 2, backgroundColor: 'action.hover' }}>
          <Typography variant="body2" color="text.secondary">
            Load:{' '}
            {tierFilter === 5 && (
              <>
                <Box component="span" sx={{ color: tierColor(5), fontWeight: 600 }}>{groupedRows.t5Count.toLocaleString()} × T5</Box>
                {' · '}
              </>
            )}
            <Box component="span" sx={{ color: tierColor(4), fontWeight: 600 }}>{groupedRows.t4Count.toLocaleString()} × T4</Box>
            {' · '}
            <Box component="span" sx={{ color: tierColor(3), fontWeight: 600 }}>{groupedRows.t3Count.toLocaleString()} × T3</Box>
            {' · '}
            <Box component="span" sx={{ color: tierColor(2), fontWeight: 600 }}>{groupedRows.t2Count.toLocaleString()} × T2</Box>
            {' raw'}
          </Typography>
        </Paper>
      )}

      {!loading && (
        <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
          {groupedRows.cityOrder.reduce((n, c) => n + (groupedRows.byCity.get(c)?.length ?? 0), 0)} combinations shown · grouped by source city · sorted by profit/trip
        </Typography>
      )}

      {/* Table */}
      <Paper elevation={0}>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Resource</TableCell>
              <TableCell>Refine In</TableCell>
              <TableCell align="right">
                <Box component="span" sx={{ color: tierColor(2) }}>T2</Box> Buy
              </TableCell>
              <TableCell align="right">
                <Box component="span" sx={{ color: tierColor(3) }}>T3</Box> Buy
              </TableCell>
              <TableCell align="right">
                <Box component="span" sx={{ color: tierColor(4) }}>T4</Box> Buy
              </TableCell>
              {tierFilter === 5 && (
                <TableCell align="right">
                  <Box component="span" sx={{ color: tierColor(5) }}>T5</Box> Buy
                </TableCell>
              )}
              <TableCell align="right">T{tierFilter} Sell</TableCell>
              <TableCell align="right">Raw Cost</TableCell>
              <TableCell align="right">Profit/Item</TableCell>
              <TableCell align="right">Total Cost</TableCell>
              <TableCell align="right">Total Sell</TableCell>
              <TableCell align="right">Profit/Trip</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 10 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: colCount }).map((_, j) => (
                      <TableCell key={j}><Skeleton variant="text" /></TableCell>
                    ))}
                  </TableRow>
                ))
              : groupedRows.cityOrder.map((city) => {
                  const cityRows = groupedRows.byCity.get(city)!;
                  return [
                    <TableRow key={`header-${city}`}>
                      <TableCell
                        colSpan={colCount}
                        sx={{
                          backgroundColor: 'action.hover',
                          py: 0.75,
                          borderBottom: '1px solid',
                          borderColor: 'divider',
                        }}
                      >
                        <Typography variant="caption" fontWeight={700} color="text.secondary" sx={{ textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                          {city}
                        </Typography>
                      </TableCell>
                    </TableRow>,
                    ...cityRows.map((row, idx) => {
                      const hasData = row.raw_cost > 0 && row.refined_sell_price > 0;
                      const profitable = row.profit_per_item > 0;
                      return (
                        <TableRow key={`${city}-${idx}`} sx={{ opacity: hasData ? 1 : 0.45 }}>
                          <TableCell>
                            <Typography variant="body2" fontWeight={600}>
                              {row.resource_type}
                            </Typography>
                          </TableCell>

                          <TableCell>
                            <Typography variant="body2" color="text.secondary">
                              {row.bonus_city_name}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', color: tierColor(2) }}>
                              {fmtPrice(row.raw_t2_buy_price)}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', color: tierColor(3) }}>
                              {fmtPrice(row.raw_t3_buy_price)}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', color: tierColor(4) }}>
                              {fmtPrice(row.raw_t4_buy_price)}
                            </Typography>
                          </TableCell>

                          {tierFilter === 5 && (
                            <TableCell align="right">
                              <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', color: tierColor(5) }}>
                                {fmtPrice(row.raw_t5_buy_price)}
                              </Typography>
                            </TableCell>
                          )}

                          <TableCell align="right">
                            <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {fmtPrice(row.refined_sell_price)}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {row.raw_cost > 0 ? Math.round(row.raw_cost).toLocaleString() : '—'}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography
                              variant="body2"
                              fontWeight={600}
                              color={!hasData ? 'text.disabled' : profitable ? 'success.main' : 'error.main'}
                              sx={{ fontVariantNumeric: 'tabular-nums' }}
                            >
                              {hasData ? `${profitable ? '+' : ''}${Math.round(row.profit_per_item).toLocaleString()}` : '—'}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {row.raw_cost > 0 ? fmtSilver(row.raw_cost * row.batches_per_trip) : '—'}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography variant="body2" color="text.secondary" sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {row.refined_sell_price > 0 ? fmtSilver(row.refined_sell_price * 0.935 * row.batches_per_trip) : '—'}
                            </Typography>
                          </TableCell>

                          <TableCell align="right">
                            <Typography
                              variant="body2"
                              fontWeight={700}
                              color={!hasData ? 'text.disabled' : profitable ? 'success.main' : 'error.main'}
                              sx={{ fontVariantNumeric: 'tabular-nums' }}
                            >
                              {hasData ? `${profitable ? '+' : ''}${fmtSilver(row.profit_per_trip)}` : '—'}
                            </Typography>
                          </TableCell>
                        </TableRow>
                      );
                    }),
                  ];
                })}
          </TableBody>
        </Table>
      </Paper>
    </Container>
  );
}

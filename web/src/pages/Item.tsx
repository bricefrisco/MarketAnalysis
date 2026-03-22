import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Container, Typography, Paper, Button,
  Table, TableHead, TableBody, TableRow, TableCell,
  Chip, Skeleton, Alert, ToggleButtonGroup, ToggleButton,
} from '@mui/material';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import BarChartIcon from '@mui/icons-material/BarChart';
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid,
  Tooltip, Legend, ResponsiveContainer,
} from 'recharts';
import { type MarketPrice, type MarketHistoryPoint, fetchItemPrices, fetchItemHistory } from '../api';
import { parseItemMeta, qualityLabel } from '../itemUtils';

const CITY_COLORS: Record<string, string> = {
  'Thetford':      '#f87171',
  'Lymhurst':      '#4ade80',
  'Bridgewatch':   '#fb923c',
  'Martlock':      '#60a5fa',
  'Fort Sterling': '#c084fc',
};

const CITY_CHIP: Record<string, 'error' | 'success' | 'warning' | 'info' | 'secondary'> = {
  'Thetford':      'error',
  'Lymhurst':      'success',
  'Bridgewatch':   'warning',
  'Martlock':      'info',
  'Fort Sterling': 'secondary',
};

const TIMESCALES = [
  { label: 'Hourly', value: 0 },
  { label: 'Daily',  value: 1 },
  { label: 'Weekly', value: 2 },
];

function pivotHistory(points: MarketHistoryPoint[]) {
  const byTime = new Map<number, Record<string, number | undefined>>();
  for (const p of points) {
    if (!byTime.has(p.timestamp)) byTime.set(p.timestamp, { time: p.timestamp });
    byTime.get(p.timestamp)![p.city] = Math.round(p.per_item);
  }
  return Array.from(byTime.values()).sort((a, b) => (a.time as number) - (b.time as number));
}

function getCities(points: MarketHistoryPoint[]): string[] {
  return [...new Set(points.map((p) => p.city))].sort();
}

function formatTs(ts: number, timescale: number): string {
  const d = new Date(ts * 1000);
  if (timescale === 0)
    return d.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

export function Item() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [prices, setPrices] = useState<MarketPrice[]>([]);
  const [history, setHistory] = useState<MarketHistoryPoint[]>([]);
  const [timescale, setTimescale] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    Promise.all([fetchItemPrices(id), fetchItemHistory(id, timescale)])
      .then(([p, h]) => { setPrices(p); setHistory(h); })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [id, timescale]);

  const chartData = pivotHistory(history);
  const cities = getCities(history);

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 4 }}>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate('/')}
          variant="outlined"
          size="small"
          sx={{ mt: 0.5, flexShrink: 0 }}
        >
          Back
        </Button>
        <Box>
          <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5 }}>
            {id && (() => {
              const meta = parseItemMeta(id);
              return (
                <Typography variant="h5" fontWeight={800} color="text.secondary"
                  sx={{ letterSpacing: '-0.02em' }}>
                  {meta.label}
                </Typography>
              );
            })()}
            <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em" color="text.primary">
              {id}
            </Typography>
          </Box>
          <Typography variant="body2" color="text.secondary">Market data</Typography>
        </Box>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 3 }}>{error}</Alert>}

      {/* Current prices */}
      <Typography variant="subtitle1" fontWeight={600} color="text.secondary"
        sx={{ mb: 1.5, textTransform: 'uppercase', fontSize: '0.75rem', letterSpacing: '0.08em' }}>
        Current Listings
      </Typography>
      <Paper elevation={0} sx={{ mb: 4 }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>City</TableCell>
              <TableCell align="center">Quality</TableCell>
              <TableCell align="right">Min Price</TableCell>
              <TableCell align="right">Avg Price</TableCell>
              <TableCell align="right">Supply</TableCell>
              <TableCell align="right">Orders</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 5 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 6 }).map((_, j) => (
                      <TableCell key={j}><Skeleton variant="text" /></TableCell>
                    ))}
                  </TableRow>
                ))
              : prices.length === 0
              ? (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Typography color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                      No listings found
                    </Typography>
                  </TableCell>
                </TableRow>
              )
              : prices.map((p, idx) => (
                  <TableRow key={idx} sx={{
                    backgroundColor: p.PriceClass === 'cheapest'
                      ? 'rgba(74, 222, 128, 0.05)'
                      : p.PriceClass === 'expensive'
                      ? 'rgba(248, 113, 113, 0.05)'
                      : undefined,
                  }}>
                    <TableCell>
                      <Chip
                        label={p.City}
                        size="small"
                        color={CITY_CHIP[p.City] ?? 'default'}
                        variant="outlined"
                        sx={{ height: 20, fontSize: '0.7rem' }}
                      />
                    </TableCell>
                    <TableCell align="center">
                      <Typography variant="caption" fontWeight={600} color="text.secondary">
                        {qualityLabel(p.Quality)}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" fontWeight={700} color="success.main"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {p.MinPrice.toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" color="text.secondary"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {Math.round(p.AvgPrice).toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" color="text.secondary"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {p.Supply.toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" color="text.secondary">
                        {p.NumOrders}
                      </Typography>
                    </TableCell>
                  </TableRow>
                ))
            }
          </TableBody>
        </Table>
      </Paper>

      {/* Price history chart */}
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <BarChartIcon sx={{ color: 'primary.main', fontSize: 20 }} />
          <Typography variant="subtitle1" fontWeight={600} color="text.secondary"
            sx={{ textTransform: 'uppercase', fontSize: '0.75rem', letterSpacing: '0.08em' }}>
            Price History
          </Typography>
        </Box>
        <ToggleButtonGroup
          value={timescale}
          exclusive
          size="small"
          onChange={(_, v) => { if (v !== null) setTimescale(v); }}
        >
          {TIMESCALES.map((ts) => (
            <ToggleButton key={ts.value} value={ts.value} sx={{ px: 2, py: 0.5, fontSize: '0.8rem' }}>
              {ts.label}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      </Box>

      <Paper elevation={0} sx={{ p: 3 }}>
        {loading ? (
          <Skeleton variant="rectangular" height={300} sx={{ borderRadius: 1 }} />
        ) : chartData.length === 0 ? (
          <Typography color="text.secondary" sx={{ textAlign: 'center', py: 4 }}>
            No price history available
          </Typography>
        ) : (
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={chartData} margin={{ top: 4, right: 16, left: 8, bottom: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#2a2a35" />
              <XAxis
                dataKey="time"
                tickFormatter={(v) => formatTs(v, timescale)}
                tick={{ fill: '#8b8b9e', fontSize: 11 }}
                axisLine={{ stroke: '#2a2a35' }}
                tickLine={false}
                minTickGap={56}
              />
              <YAxis
                tickFormatter={(v) => v.toLocaleString()}
                tick={{ fill: '#8b8b9e', fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                width={72}
              />
              <Tooltip
                contentStyle={{ backgroundColor: '#18181f', border: '1px solid #2a2a35', borderRadius: 8 }}
                labelStyle={{ color: '#8b8b9e', fontSize: 12, marginBottom: 4 }}
                itemStyle={{ fontSize: 12 }}
                formatter={(v) => (v != null ? Number(v).toLocaleString() + ' silver' : '—')}
                labelFormatter={(v) => formatTs(v as number, timescale)}
              />
              <Legend wrapperStyle={{ fontSize: 12, paddingTop: 12 }} />
              {cities.map((city) => (
                <Line
                  key={city}
                  type="monotone"
                  dataKey={city}
                  stroke={CITY_COLORS[city] ?? '#888'}
                  dot={false}
                  strokeWidth={2}
                  connectNulls
                />
              ))}
            </LineChart>
          </ResponsiveContainer>
        )}
      </Paper>
    </Container>
  );
}

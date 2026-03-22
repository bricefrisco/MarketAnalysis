import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box, Container, Typography, Paper,
  Table, TableHead, TableBody, TableRow, TableCell,
  Skeleton, Alert, Chip,
} from '@mui/material';
import TrendingUpIcon from '@mui/icons-material/TrendingUp';
import { type Order, fetchRecentOrders } from '../api';
import { parseItemMeta, qualityLabel } from '../itemUtils';

type ItemsMap = Record<string, string>;

const CITY_COLORS: Record<string, 'default' | 'primary' | 'secondary' | 'error' | 'info' | 'success' | 'warning'> = {
  'Thetford': 'error',
  'Lymhurst': 'success',
  'Bridgewatch': 'warning',
  'Martlock': 'info',
  'Fort Sterling': 'secondary',
};

function timeAgo(dateStr: string): string {
  const diff = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function Home() {
  const [orders, setOrders] = useState<Order[]>([]);
  const [items, setItems] = useState<ItemsMap>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    fetch('/api/items')
      .then((r) => r.json())
      .then(setItems)
      .catch(() => {});
  }, []);

  useEffect(() => {
    fetchRecentOrders(100)
      .then((data) => {
        const sorted = [...data].sort((a, b) => {
          const pctA = a.weekly_avg != null && a.weekly_avg > 0
            ? (a.weekly_avg - a.unit_price_silver) / a.weekly_avg
            : -Infinity;
          const pctB = b.weekly_avg != null && b.weekly_avg > 0
            ? (b.weekly_avg - b.unit_price_silver) / b.weekly_avg
            : -Infinity;
          return pctB - pctA;
        });
        setOrders(sorted);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 1 }}>
        <TrendingUpIcon sx={{ color: 'primary.main' }} />
        <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em">
          Live Market Orders
        </Typography>
      </Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Most recent sell orders across all Royal Continent cities
      </Typography>

      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}

      <Paper elevation={0}>
        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Item</TableCell>
              <TableCell>City</TableCell>
              <TableCell align="center">Quality</TableCell>
              <TableCell align="right">Price</TableCell>
              <TableCell align="right">7d Avg</TableCell>
              <TableCell align="right">Discount</TableCell>
              <TableCell align="right">Supply</TableCell>
              <TableCell align="right">Seen</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 12 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 8 }).map((_, j) => (
                      <TableCell key={j}><Skeleton variant="text" /></TableCell>
                    ))}
                  </TableRow>
                ))
              : orders.map((order, idx) => (
                  <TableRow
                    key={idx}
                    hover
                    sx={{ cursor: 'pointer' }}
                    onClick={() => navigate(`/item/${order.item_type_id}`)}
                  >
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {(() => {
                          const meta = parseItemMeta(order.item_type_id);
                          return (
                            <Typography variant="caption" fontWeight={700}
                              sx={{ color: 'text.secondary', whiteSpace: 'nowrap', minWidth: 28 }}>
                              {meta.label}
                            </Typography>
                          );
                        })()}
                        <Typography variant="body2" fontWeight={500} color="text.primary"
                          sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {items[order.item_type_id] || order.item_type_id}
                        </Typography>
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={order.city}
                        size="small"
                        color={CITY_COLORS[order.city] ?? 'default'}
                        variant="outlined"
                        sx={{ height: 20, fontSize: '0.7rem' }}
                      />
                    </TableCell>
                    <TableCell align="center">
                      <Typography variant="caption" fontWeight={600} color="text.secondary">
                        {qualityLabel(order.quality_level)}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" fontWeight={600} color="success.main"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {order.unit_price_silver.toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" color="text.secondary"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {order.weekly_avg != null ? Math.round(order.weekly_avg).toLocaleString() : '—'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      {order.weekly_avg != null && order.weekly_avg > 0 ? (() => {
                        const pct = (order.weekly_avg - order.unit_price_silver) / order.weekly_avg * 100;
                        const color = pct > 0 ? 'success.main' : pct < 0 ? 'error.main' : 'text.secondary';
                        return (
                          <Typography variant="body2" fontWeight={600} color={color}
                            sx={{ fontVariantNumeric: 'tabular-nums' }}>
                            {pct > 0 ? '-' : pct < 0 ? '+' : ''}{Math.abs(pct).toFixed(1)}%
                          </Typography>
                        );
                      })() : (
                        <Typography variant="body2" color="text.disabled">—</Typography>
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="body2" color="text.secondary"
                        sx={{ fontVariantNumeric: 'tabular-nums' }}>
                        {order.amount.toLocaleString()}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography variant="caption" color="text.secondary">
                        {timeAgo(order.captured_at)}
                      </Typography>
                    </TableCell>
                  </TableRow>
                ))
            }
          </TableBody>
        </Table>
      </Paper>
    </Container>
  );
}

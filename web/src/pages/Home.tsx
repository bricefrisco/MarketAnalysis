import { useEffect, useState } from 'react';
import {
  Box, Container, Typography, Paper, Button,
  Table, TableHead, TableBody, TableRow, TableCell,
  Skeleton, Alert, Chip, TablePagination,
  Dialog, DialogTitle, DialogContent, DialogActions,
} from '@mui/material';
import TrendingUpIcon from '@mui/icons-material/TrendingUp';
import DeleteIcon from '@mui/icons-material/Delete';
import { type Order, type OrdersResponse, fetchRecentOrders, clearData } from '../api';
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
  const [response, setResponse] = useState<OrdersResponse | null>(null);
  const [items, setItems] = useState<ItemsMap>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(25);
  const [showClearDialog, setShowClearDialog] = useState(false);
  const [clearing, setClearing] = useState(false);

  useEffect(() => {
    fetch('/api/items')
      .then((r) => r.json())
      .then(setItems)
      .catch(() => {});
  }, []);

  useEffect(() => {
    setLoading(true);
    fetchRecentOrders(page + 1, pageSize)
      .then(setResponse)
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [page, pageSize]);

  const handleClear = async () => {
    setClearing(true);
    try {
      await clearData();
      setShowClearDialog(false);
      setPage(0);
      setResponse(null);
      setLoading(true);
      const result = await fetchRecentOrders(1, pageSize);
      setResponse(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to clear data');
    } finally {
      setClearing(false);
    }
  };

  const orders: Order[] = response?.orders ?? [];

  return (
    <Container maxWidth="xl" sx={{ py: 4 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1.5, mb: 1 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <TrendingUpIcon sx={{ color: 'primary.main' }} />
          <Typography variant="h5" fontWeight={700} letterSpacing="-0.02em">
            Live Market Orders
          </Typography>
        </Box>
        <Button
          variant="outlined"
          color="error"
          size="small"
          startIcon={<DeleteIcon />}
          onClick={() => setShowClearDialog(true)}
        >
          Clear Data
        </Button>
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
              <TableCell align="right">4w Avg</TableCell>
              <TableCell align="right">Profit</TableCell>
              <TableCell align="right">Profit %</TableCell>
              <TableCell align="right">Supply</TableCell>
              <TableCell align="right">Seen</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 12 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 9 }).map((_, j) => (
                      <TableCell key={j}><Skeleton variant="text" /></TableCell>
                    ))}
                  </TableRow>
                ))
              : orders.map((order, idx) => (
                  <TableRow key={idx}>
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
                        {order.monthly_avg != null ? Math.round(order.monthly_avg).toLocaleString() : '—'}
                      </Typography>
                    </TableCell>
                    {(() => {
                      const profit = order.profit;
                      const profitPct = order.profit_pct;
                      if (profit == null || profitPct == null) {
                        return (
                          <>
                            <TableCell align="right"><Typography variant="body2" color="text.disabled">—</Typography></TableCell>
                            <TableCell align="right"><Typography variant="body2" color="text.disabled">—</Typography></TableCell>
                          </>
                        );
                      }
                      const color = profit > 0 ? 'success.main' : profit < 0 ? 'error.main' : 'text.secondary';
                      return (
                        <>
                          <TableCell align="right">
                            <Typography variant="body2" fontWeight={600} color={color}
                              sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {profit > 0 ? '+' : ''}{Math.round(profit).toLocaleString()}
                            </Typography>
                          </TableCell>
                          <TableCell align="right">
                            <Typography variant="body2" fontWeight={600} color={color}
                              sx={{ fontVariantNumeric: 'tabular-nums' }}>
                              {profit > 0 ? '+' : ''}{profitPct.toFixed(1)}%
                            </Typography>
                          </TableCell>
                        </>
                      );
                    })()}
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
        <TablePagination
          component="div"
          count={response?.total ?? 0}
          page={page}
          onPageChange={(_, newPage) => setPage(newPage)}
          rowsPerPage={pageSize}
          onRowsPerPageChange={(e) => { setPageSize(parseInt(e.target.value, 10)); setPage(0); }}
          rowsPerPageOptions={[25, 50, 100, 200]}
        />
      </Paper>

      <Dialog open={showClearDialog} onClose={() => setShowClearDialog(false)}>
        <DialogTitle>Clear Data</DialogTitle>
        <DialogContent>
          <Typography>Are you sure you want to delete all market data? This action cannot be undone.</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowClearDialog(false)}>Cancel</Button>
          <Button onClick={handleClear} color="error" variant="contained" disabled={clearing}>
            {clearing ? 'Clearing...' : 'Clear'}
          </Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
}

import { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  InputBase, Paper, List, ListItemButton, ListItemText,
  CircularProgress, Box, Typography,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import { type Item, searchItems } from '../api';

export function Search() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<Item[]>([]);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const containerRef = useRef<HTMLDivElement>(null);

  const handleSearch = async (q: string) => {
    setQuery(q);
    if (q.length < 2) { setResults([]); setOpen(false); return; }
    setLoading(true);
    setOpen(true);
    try {
      const items = await searchItems(q);
      setResults(items || []);
    } finally {
      setLoading(false);
    }
  };

  // Close on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const handleSelect = (id: string) => {
    setOpen(false);
    setQuery('');
    setResults([]);
    navigate(`/item/${id}`);
  };

  return (
    <Box ref={containerRef} sx={{ position: 'relative', width: { xs: '100%', sm: 360 } }}>
      <Paper elevation={0} sx={{
        display: 'flex', alignItems: 'center', gap: 1,
        px: 1.5, py: 0.75,
        backgroundColor: 'rgba(255,255,255,0.05)',
        border: '1px solid',
        borderColor: open ? 'primary.main' : 'divider',
        borderRadius: 2,
        transition: 'border-color 0.2s',
      }}>
        <SearchIcon sx={{ color: 'text.secondary', fontSize: 18 }} />
        <InputBase
          value={query}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="Search items…"
          sx={{ flex: 1, fontSize: '0.875rem', color: 'text.primary' }}
        />
        {loading && <CircularProgress size={14} sx={{ color: 'primary.main' }} />}
      </Paper>

      {open && (
        <Paper elevation={8} sx={{
          position: 'absolute', top: 'calc(100% + 6px)', left: 0, right: 0,
          zIndex: 200, maxHeight: 380, overflow: 'auto',
          border: '1px solid', borderColor: 'divider',
        }}>
          {results.length > 0 ? (
            <List dense disablePadding>
              {results.slice(0, 50).map((item) => (
                <ListItemButton key={item.id} onClick={() => handleSelect(item.id)}
                  sx={{ py: 1, '&:hover': { backgroundColor: 'rgba(124,106,247,0.08)' } }}>
                  <ListItemText
                    primary={item.name}
                    secondary={item.id}
                    primaryTypographyProps={{ fontSize: '0.875rem', color: 'text.primary' }}
                    secondaryTypographyProps={{ fontSize: '0.72rem', color: 'text.secondary' }}
                  />
                </ListItemButton>
              ))}
            </List>
          ) : (
            <Typography sx={{ p: 2, color: 'text.secondary', fontSize: '0.875rem', textAlign: 'center' }}>
              No items found
            </Typography>
          )}
        </Paper>
      )}
    </Box>
  );
}

import { useState } from 'react';
import { ThemeProvider, CssBaseline, AppBar, Toolbar, Typography, Box, Tabs, Tab } from '@mui/material';
import ShieldIcon from '@mui/icons-material/Shield';
import theme from './theme';
import { Crafting } from './pages/Crafting';
import { Refining } from './pages/Refining';

function App() {
  const [tab, setTab] = useState(0);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <AppBar position="sticky" elevation={0} sx={{
        backgroundColor: 'background.paper',
        borderBottom: '1px solid',
        borderColor: 'divider',
      }}>
        <Toolbar sx={{ gap: 1.5 }}>
          <ShieldIcon sx={{ color: 'primary.main' }} />
          <Typography variant="h6" sx={{ color: 'text.primary', letterSpacing: '-0.02em' }}>
            Albion Tools
          </Typography>
          <Tabs
            value={tab}
            onChange={(_, v) => setTab(v)}
            sx={{ ml: 2 }}
            textColor="primary"
            indicatorColor="primary"
          >
            <Tab label="Crafting" sx={{ fontSize: '0.85rem', minHeight: 64 }} />
            <Tab label="Refining" sx={{ fontSize: '0.85rem', minHeight: 64 }} />
          </Tabs>
        </Toolbar>
      </AppBar>
      <Box sx={{ minHeight: 'calc(100vh - 64px)', backgroundColor: 'background.default' }}>
        {tab === 0 && <Crafting />}
        {tab === 1 && <Refining />}
      </Box>
    </ThemeProvider>
  );
}

export default App;

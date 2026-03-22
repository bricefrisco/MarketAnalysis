import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider, CssBaseline, AppBar, Toolbar, Typography, Box } from '@mui/material';
import ShieldIcon from '@mui/icons-material/Shield';
import theme from './theme';
import { Search } from './pages/Search';
import { Home } from './pages/Home';
import { Item } from './pages/Item';

function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <BrowserRouter>
        <AppBar position="sticky" elevation={0} sx={{
          backgroundColor: 'background.paper',
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}>
          <Toolbar sx={{ gap: 2 }}>
            <ShieldIcon sx={{ color: 'primary.main' }} />
            <Typography variant="h6" sx={{ color: 'text.primary', letterSpacing: '-0.02em', mr: 'auto' }}>
              Albion Market
            </Typography>
            <Search />
          </Toolbar>
        </AppBar>
        <Box sx={{ minHeight: 'calc(100vh - 64px)', backgroundColor: 'background.default' }}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/item/:id" element={<Item />} />
          </Routes>
        </Box>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;

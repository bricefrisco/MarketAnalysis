import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom';
import { ThemeProvider, CssBaseline, AppBar, Toolbar, Typography, Box, Button } from '@mui/material';
import ShieldIcon from '@mui/icons-material/Shield';
import RadarIcon from '@mui/icons-material/Radar';
import BuildIcon from '@mui/icons-material/Build';
import theme from './theme';
import { Search } from './pages/Search';
import { Home } from './pages/Home';
import { Scanner } from './pages/Scanner';
import { Crafting } from './pages/Crafting';

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
            <Typography variant="h6" sx={{ color: 'text.primary', letterSpacing: '-0.02em' }}>
              Albion Market
            </Typography>
            <Box sx={{ display: 'flex', gap: 0.5, mr: 'auto', ml: 2 }}>
              <Button component={NavLink} to="/" end size="small"
                sx={{ color: 'text.secondary', '&.active': { color: 'primary.main' } }}>
                Market
              </Button>
              <Button component={NavLink} to="/scanner" size="small"
                startIcon={<RadarIcon sx={{ fontSize: '16px !important' }} />}
                sx={{ color: 'text.secondary', '&.active': { color: 'primary.main' } }}>
                Scanner
              </Button>
              <Button component={NavLink} to="/crafting" size="small"
                startIcon={<BuildIcon sx={{ fontSize: '16px !important' }} />}
                sx={{ color: 'text.secondary', '&.active': { color: 'primary.main' } }}>
                Crafting
              </Button>
            </Box>
            <Search />
          </Toolbar>
        </AppBar>
        <Box sx={{ minHeight: 'calc(100vh - 64px)', backgroundColor: 'background.default' }}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/scanner" element={<Scanner />} />
            <Route path="/crafting" element={<Crafting />} />
          </Routes>
        </Box>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;

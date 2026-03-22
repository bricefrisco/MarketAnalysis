import { createTheme } from '@mui/material/styles';

const theme = createTheme({
  palette: {
    mode: 'dark',
    primary: { main: '#7c6af7' },
    secondary: { main: '#4ecdc4' },
    background: {
      default: '#0f0f13',
      paper: '#18181f',
    },
    text: {
      primary: '#f1f1f5',
      secondary: '#8b8b9e',
    },
    divider: '#2a2a35',
  },
  typography: {
    fontFamily: `'Inter', system-ui, -apple-system, sans-serif`,
    h1: { fontWeight: 700, letterSpacing: '-0.03em' },
    h2: { fontWeight: 600, letterSpacing: '-0.02em' },
    h6: { fontWeight: 600 },
  },
  shape: { borderRadius: 10 },
  components: {
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          border: '1px solid #2a2a35',
        },
      },
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          '& .MuiTableCell-root': {
            color: '#8b8b9e',
            fontSize: '0.75rem',
            fontWeight: 600,
            textTransform: 'uppercase',
            letterSpacing: '0.06em',
            borderBottom: '1px solid #2a2a35',
          },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: { borderBottom: '1px solid #1e1e28' },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          '&:last-child td': { borderBottom: 0 },
          '&:hover': { backgroundColor: 'rgba(124, 106, 247, 0.05)' },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: { fontWeight: 600, fontSize: '0.72rem' },
      },
    },
    MuiButton: {
      styleOverrides: {
        root: { textTransform: 'none', fontWeight: 600 },
      },
    },
    MuiInputBase: {
      styleOverrides: {
        root: { borderRadius: 10 },
      },
    },
    MuiOutlinedInput: {
      styleOverrides: {
        notchedOutline: { borderColor: '#2a2a35' },
      },
    },
    MuiToggleButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 500,
          borderColor: '#2a2a35',
          color: '#8b8b9e',
          '&.Mui-selected': {
            backgroundColor: 'rgba(124, 106, 247, 0.15)',
            color: '#7c6af7',
            borderColor: '#7c6af7',
          },
        },
      },
    },
  },
});

export default theme;

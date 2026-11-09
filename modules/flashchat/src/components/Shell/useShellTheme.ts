import { SettingsContext } from 'contexts/SettingsContext'
import { useContext, useMemo } from 'react'
import { createTheme } from '@mui/material/styles'
import { useHostState } from 'config/host'

export const useShellTheme = () => {
  const { getUserSettings } = useContext(SettingsContext)
  const { colorMode } = getUserSettings()
  const host = useHostState()
  const mode = window.__winoneState ? host.theme : colorMode

  const theme = useMemo(
    () =>
      // NOTE: You can make theme customizations here. It is recommended to use
      // the default theme viewer as a reference:
      // https://mui.com/material-ui/customization/default-theme/
      createTheme({
        palette: {
          mode,
          primary: { main: mode === 'dark' ? '#e5e5e5' : '#262626' },
          secondary: { main: mode === 'dark' ? '#bcbcbc' : '#535353' },
          background: { default: mode === 'dark' ? '#191919' : '#fcfcfc', paper: mode === 'dark' ? '#232323' : '#ffffff' },
        },
        typography: {
          fontFamily: '"Segoe UI", "Microsoft YaHei UI", "Microsoft YaHei", sans-serif',
          fontSize: 14, fontWeightBold: 600,
          // Match the native window's hierarchy instead of MUI's large
          // marketing headings, including the upstream Markdown help pages.
          h1: { fontSize: 30, fontWeight: 500 },
          h2: { fontSize: 26, fontWeight: 500 },
          h3: { fontSize: 22, fontWeight: 500 },
          h4: { fontSize: 20, fontWeight: 500 },
          h5: { fontSize: 18, fontWeight: 500 },
          h6: { fontSize: 16, fontWeight: 500 },
          button: { textTransform: 'none' },
        },
        shape: { borderRadius: 8 },
        components: {
          MuiButton: { defaultProps: { disableElevation: true }, styleOverrides: { root: { minHeight: 36 } } },
          MuiAppBar: { styleOverrides: { root: { boxShadow: 'none', backgroundImage: 'none', backgroundColor: mode === 'dark' ? '#191919' : '#fcfcfc', color: mode === 'dark' ? '#ededed' : '#262626', borderBottom: '1px solid', borderColor: mode === 'dark' ? '#363636' : '#e5e5e5' } } },
          MuiPaper: { styleOverrides: { root: { backgroundImage: 'none', boxShadow: 'none' } } },
          MuiAccordion: { styleOverrides: { root: { border: '1px solid', borderColor: mode === 'dark' ? '#363636' : '#d8d8d8' } } },
          MuiFab: { styleOverrides: { root: { boxShadow: 'none', '&:hover': { boxShadow: 'none' } } } },
          MuiTooltip: { defaultProps: { arrow: true } },
          MuiCssBaseline: { styleOverrides: { body: { overflowWrap: 'anywhere' }, '*': { minWidth: 0 }, '@media (prefers-reduced-motion: reduce)': { '*, *::before, *::after': { animationDuration: '0.01ms !important', transitionDuration: '0.01ms !important' } } } },
        },
      }),
    [mode, host.language]
  )

  return theme
}

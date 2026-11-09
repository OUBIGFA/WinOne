import { t, useLanguage } from 'config/i18n'
import { PropsWithChildren, useContext } from 'react'
import { Link } from 'react-router-dom'
import useTheme from '@mui/material/styles/useTheme'
import Box from '@mui/material/Box'
import MuiDrawer from '@mui/material/Drawer'
import List from '@mui/material/List'
import MuiLink from '@mui/material/Link'
import Typography from '@mui/material/Typography'
import Divider from '@mui/material/Divider'
import IconButton from '@mui/material/IconButton'
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Home from '@mui/icons-material/Home'
import SettingsApplications from '@mui/icons-material/SettingsRounded'
import QuestionMark from '@mui/icons-material/QuestionMark'
import Brightness4Icon from '@mui/icons-material/Brightness4'
import Brightness7Icon from '@mui/icons-material/Brightness7'
import ReportIcon from '@mui/icons-material/Report'

import { routes } from 'config/routes'
import { SettingsContext } from 'contexts/SettingsContext'
import { ColorMode } from 'core/models/settings'

// Identify the imported source, never the unrelated enclosing WinOne commit.
const commit = { hash: '708fefdcc3ac0f5fc2ef2ca9ce1cc1b6a9f46c74', shortHash: '708fefdc' }

export const drawerWidth = 240

export interface DrawerProps extends PropsWithChildren {
  isDrawerOpen: boolean
  onDrawerClose: () => void
  overlay?: boolean
}

export const Drawer = ({ isDrawerOpen, onDrawerClose, overlay = false }: DrawerProps) => {
  useLanguage()

  const theme = useTheme()
  const settingsContext = useContext(SettingsContext)
  const colorMode = settingsContext.getUserSettings().colorMode

  const handleColorModeToggleClick = () => {
    if (window.__winoneState) {
      window.location.href = 'winone-flashchat:appearance'
      return
    }
    const newMode =
      colorMode === ColorMode.LIGHT ? ColorMode.DARK : ColorMode.LIGHT

    settingsContext.updateUserSettings({ colorMode: newMode })
  }

  return (
    <MuiDrawer
      sx={{
        width: overlay ? 0 : drawerWidth,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: drawerWidth,
          boxSizing: 'border-box',
        },
      }}
      variant={overlay ? 'temporary' : 'persistent'}
      ModalProps={{ keepMounted: true }}
      onClose={onDrawerClose}
      anchor="left"
      open={isDrawerOpen}
    >
      <Box
        sx={() => ({
          display: 'flex',
          alignItems: 'center',
          padding: theme.spacing(0, 1),
          // necessary for drawer content to be pushed below app bar
          ...theme.mixins.toolbar,
          justifyContent: 'flex-end',
        })}
      >
        <IconButton onClick={onDrawerClose} aria-label={t("Close menu")}>
          {theme.direction === 'ltr' ? (
            <ChevronLeftIcon />
          ) : (
            <ChevronRightIcon />
          )}
        </IconButton>
      </Box>
      <Divider />
      <Box component="nav" aria-label={t("Navigation menu")}>
        <List>
          <ListItem disablePadding>
            <ListItemButton component={Link} to={routes.ROOT}>
              <ListItemIcon>
                <Home />
              </ListItemIcon>
              <ListItemText primary={t("Home")} />
            </ListItemButton>
          </ListItem>
          <ListItem disablePadding>
            <ListItemButton component={Link} to={routes.SETTINGS}>
              <ListItemIcon>
                <SettingsApplications />
              </ListItemIcon>
              <ListItemText primary={t("Settings")} />
            </ListItemButton>
          </ListItem>
          <ListItem disablePadding>
            <ListItemButton component={Link} to={routes.ABOUT}>
              <ListItemIcon>
                <QuestionMark />
              </ListItemIcon>
              <ListItemText primary={t("About")} />
            </ListItemButton>
          </ListItem>
          <ListItem disablePadding>
            <ListItemButton component={Link} to={routes.DISCLAIMER}>
              <ListItemIcon>
                <ReportIcon />
              </ListItemIcon>
              <ListItemText primary={t("Disclaimer")} />
            </ListItemButton>
          </ListItem>
          <ListItem disablePadding>
            <ListItemButton onClick={handleColorModeToggleClick}>
              <ListItemIcon>
                {theme.palette.mode === 'dark' ? (
                  <Brightness7Icon />
                ) : (
                  <Brightness4Icon />
                )}
              </ListItemIcon>
              <ListItemText primary={window.__winoneState ? t('WinOne appearance') : t("Change theme")} />
            </ListItemButton>
          </ListItem>
        </List>
        <Divider />
        <Box sx={{ padding: 2 }}>
          <Typography variant="subtitle2">{t("Build signature:")}{' '}
            <Typography
              sx={{
                fontFamily: 'monospace',
                display: 'inline',
              }}
            >
              <MuiLink
                target="_blank"
                rel="noopener"
                href={`${import.meta.env.VITE_GITHUB_REPO}/commit/${
                  commit.hash
                }`}
              >
                {commit.shortHash}
              </MuiLink>
            </Typography>
          </Typography>
        </Box>
      </Box>
    </MuiDrawer>
  )
}

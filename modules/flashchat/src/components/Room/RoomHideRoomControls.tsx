import { t, useLanguage } from 'config/i18n'
import { useContext } from 'react'

import Box from '@mui/material/Box'
import Fab from '@mui/material/Fab'
import Tooltip from '@mui/material/Tooltip'
import { ExpandLess } from '@mui/icons-material'

import { ShellContext } from 'contexts/ShellContext'

export function RoomHideRoomControls() {
  useLanguage()

  const { setShowRoomControls } = useContext(ShellContext)

  return (
    <Box
      sx={{
        alignItems: 'center',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        px: 1,
      }}
    >
      <Tooltip title={t("Hide controls")}>
        <Fab
          color="primary"
          aria-label={t("hide controls")}
          onClick={() => setShowRoomControls(false)}
        >
          <ExpandLess />
        </Fab>
      </Tooltip>
    </Box>
  )
}

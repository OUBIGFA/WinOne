import { t, useLanguage } from 'config/i18n'
import CheckCircleOutline from '@mui/icons-material/CheckCircleOutline'
import ReportIcon from '@mui/icons-material/Report'
import CircularProgress from '@mui/material/CircularProgress'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Box } from '@mui/system'
import { useContext } from 'react'

import { ShellContext } from 'contexts/ShellContext'
import { TrackerConnection } from 'lib/ConnectionTest'

import { ConnectionTestResults as IConnectionTestResults } from './useConnectionTest'

interface ConnectionTestResultsProps {
  connectionTestResults: IConnectionTestResults
}
export const ConnectionTestResults = ({
  connectionTestResults: { hasHost, hasTURNServer, hasPublicAddress, trackerConnection },
}: ConnectionTestResultsProps) => {
  useLanguage()

  const { setIsServerConnectionFailureDialogOpen } = useContext(ShellContext)

  const handleServerConnectionFailedMessageClick = () => {
    setIsServerConnectionFailureDialogOpen(true)
  }

  if (trackerConnection === TrackerConnection.FAILED) {
    return (
      <Typography
        variant="subtitle2"
        sx={{ cursor: 'pointer' }}
        onClick={handleServerConnectionFailedMessageClick}
      >
        <Box
          sx={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}
        >
          <ReportIcon color="error" sx={{ mr: 1 }} />
          <span>{t("Server connection failed")}</span>
        </Box>
      </Typography>
    )
  }

  if (trackerConnection !== TrackerConnection.CONNECTED) {
    return (
      <Typography variant="subtitle2">
        <Box
          sx={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}
        >
          <CircularProgress size={16} sx={{ mr: 1.5 }} />
          <span>{t("Searching for servers...")}</span>
        </Box>
      </Typography>
    )
  }

  // Candidate gathering describes available routes, not a completed call.
  const hasFullConnectivity = hasTURNServer

  if (hasFullConnectivity) {
    return (
      <Tooltip title={t("A relay address is available. The connection still depends on the other participant and the network.")}>
        <Typography variant="subtitle2">
          <Typography
            component="span"
            sx={theme => ({ color: theme.palette.success.main })}
          >
            <CheckCircleOutline sx={{ fontSize: 'small' }} />
          </Typography>{' '}{t("Relay available")}</Typography>
      </Tooltip>
    )
  } else if (hasPublicAddress || hasHost) {
    return (
      <Tooltip title={t(hasPublicAddress ? "A public address was found. Direct connections may work; a relay is not available." : "Only local addresses were found. Internet connectivity has not been confirmed.")}>
        <Typography variant="subtitle2">
          <Typography
            component="span"
            sx={theme => ({ color: theme.palette.warning.main })}
          >
            <ReportIcon sx={{ fontSize: 'small' }} />
          </Typography>{' '}{t(hasPublicAddress ? "Direct connection available" : "Local addresses only")}</Typography>
      </Tooltip>
    )
  } else {
    return (
      <Tooltip title={t("No usable connection address has been found yet.")}>
        <Typography variant="subtitle2">
          <Typography
            component="span"
            sx={theme => ({ color: theme.palette.error.main })}
          >
            <ReportIcon sx={{ fontSize: 'small' }} />
          </Typography>{' '}{t("Checking connection")}</Typography>
      </Tooltip>
    )
  }
}

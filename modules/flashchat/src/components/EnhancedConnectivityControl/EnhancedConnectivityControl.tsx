import { t, useLanguage } from 'config/i18n'
import { ChangeEvent } from 'react'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import Paper from '@mui/material/Paper'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'

interface EnhancedConnectivityControlProps {
  isEnabled: boolean
  onChange: (event: ChangeEvent, newValue: boolean) => void
  variant?: 'body2' | 'subtitle2'
  showSecondaryColor?: boolean
  elevation?: number
  sx?: object
}

export const EnhancedConnectivityControl = ({
  isEnabled,
  onChange,
  variant = 'body2',
  showSecondaryColor = false,
  elevation = 3,
  sx = { p: 2, mb: 2 },
}: EnhancedConnectivityControlProps) => {
  useLanguage()

  return (
    <Paper elevation={elevation} sx={sx}>
      <FormGroup>
        <FormControlLabel
          control={<Switch checked={isEnabled} onChange={onChange} />}
          label={t("Enhanced connectivity")}
        />
      </FormGroup>
      <Typography
        variant={variant}
        color={showSecondaryColor ? 'text.secondary' : undefined}
      >{t("Use external TURN servers to improve connection reliability. Disable this if you prefer not to connect to third-party servers.")}</Typography>
    </Paper>
  )
}

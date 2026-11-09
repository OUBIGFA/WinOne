import { t, useLanguage } from 'config/i18n'
import Box, { BoxProps } from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'

interface WholePageLoadingProps extends BoxProps {}

export const WholePageLoading = ({
  sx = [],
  ...props
}: WholePageLoadingProps) => {
  useLanguage()

  return (
    <Box
      sx={[
        {
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          position: 'absolute',
          top: 0,
          right: 0,
          bottom: 0,
          left: 0,
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
      {...props}
    >
      <CircularProgress aria-label={t("Loading")} />
    </Box>
  )
}

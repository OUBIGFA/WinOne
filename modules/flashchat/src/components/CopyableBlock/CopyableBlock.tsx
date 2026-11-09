import { t, useLanguage } from 'config/i18n'
import Box, { BoxProps } from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Fab from '@mui/material/Fab'
import ContentCopy from '@mui/icons-material/ContentCopy'
import { useContext, useRef } from 'react'
import { ShellContext } from 'contexts/ShellContext'

interface CopyableBlockProps extends BoxProps {}

export const CopyableBlock = ({ children }: CopyableBlockProps) => {
  useLanguage()

  const { showAlert } = useContext(ShellContext)
  const boxRef = useRef<HTMLDivElement>(null)

  const handleCopyClick = async () => {
    const div = boxRef?.current

    if (!div) return

    await navigator.clipboard.writeText(div.innerText)

    showAlert(t("Copied to clipboard"), { severity: 'success' })
  }

  return (
    <Box
      ref={boxRef}
      sx={{
        position: 'relative',
        '&:hover button': {
          opacity: 0.75,
        },
      }}
    >
      {children}
      <Tooltip title={t("Copy to clipboard")}>
        <Fab
          color="default"
          size="small"
          onClick={handleCopyClick}
          sx={theme => ({
            position: 'absolute',
            top: '1em',
            right: '1em',
            opacity: 0,
            transition: theme.transitions.create(['opacity', 'transform']),
          })}
        >
          <ContentCopy />
        </Fab>
      </Tooltip>
    </Box>
  )
}

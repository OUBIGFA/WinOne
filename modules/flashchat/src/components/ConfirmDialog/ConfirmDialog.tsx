import { t, useLanguage } from 'config/i18n'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import WarningIcon from '@mui/icons-material/Warning'

interface ConfirmDialogProps {
  isOpen: boolean
  onCancel: () => void
  onConfirm: () => void
}

export const ConfirmDialog = ({
  isOpen,
  onCancel,
  onConfirm,
}: ConfirmDialogProps) => {
  useLanguage()

  const handleDialogClose = () => {
    onCancel()
  }

  return (
    <Dialog
      open={isOpen}
      aria-labelledby="alert-dialog-title"
      aria-describedby="alert-dialog-description"
      onClose={handleDialogClose}
    >
      <DialogTitle id="alert-dialog-title">
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <WarningIcon
            fontSize="medium"
            sx={theme => ({
              color: theme.palette.warning.main,
              mr: theme.spacing(1),
            })}
          />{t("Are you sure?")}</Box>
      </DialogTitle>
      <DialogContent>
        <DialogContentText id="alert-dialog-description">{t("This action cannot be undone.")}</DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} autoFocus>{t("Cancel")}</Button>
        <Button onClick={onConfirm} color="error">{t("Confirm")}</Button>
      </DialogActions>
    </Dialog>
  )
}

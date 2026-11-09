import { t, useLanguage } from 'config/i18n'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Divider from '@mui/material/Divider'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import Paper from '@mui/material/Paper'
import useTheme from '@mui/material/styles/useTheme'
import Switch from '@mui/material/Switch'
import Typography from '@mui/material/Typography'
import { ChangeEvent, useContext, useEffect, useState } from 'react'
import FileReaderInput, { Result } from 'react-file-reader-input'

import { ConfirmDialog } from 'components/ConfirmDialog'
import { EnhancedConnectivityControl } from 'components/EnhancedConnectivityControl'
import { PeerNameDisplay } from 'components/PeerNameDisplay'
import { SoundSelector } from 'components/SoundSelector/SoundSelector'
import { isEnhancedConnectivityAvailable } from 'config/enhancedConnectivity'
import { SettingsContext } from 'contexts/SettingsContext'
import { ShellContext } from 'contexts/ShellContext'
import { StorageContext } from 'contexts/StorageContext'
import { notification } from 'services/Notification'
import { settings } from 'services/Settings'

import { isErrorWithMessage } from 'core/lib/type-guards'
import { NetworkSettings } from './NetworkSettings'

interface SettingsProps {
  userId: string
}

export const Settings = ({ userId }: SettingsProps) => {
  useLanguage()

  const theme = useTheme()

  const { setTitle, showAlert } = useContext(ShellContext)
  const { updateUserSettings, getUserSettings } = useContext(SettingsContext)
  const { getPersistedStorage } = useContext(StorageContext)
  const [
    isDeleteSettingsConfirmDiaglogOpen,
    setIsDeleteSettingsConfirmDiaglogOpen,
  ] = useState(false)
  const [, setIsNotificationPermissionDetermined] = useState(false)
  const {
    playSoundOnNewMessage,
    showNotificationOnNewMessage,
    showActiveTypingStatus,
    isEnhancedConnectivityEnabled,
  } = getUserSettings()

  const persistedStorage = getPersistedStorage()

  useEffect(() => {
    ;(async () => {
      await notification.requestPermission()

      // This state needs to be set to cause a rerender so that
      // areNotificationsAvailable is up-to-date.
      setIsNotificationPermissionDetermined(true)
    })()
  }, [])

  useEffect(() => {
    setTitle("Settings")
  }, [setTitle])

  const handlePlaySoundOnNewMessageChange = (
    _event: ChangeEvent,
    newPlaySoundOnNewMessage: boolean
  ) => {
    updateUserSettings({ playSoundOnNewMessage: newPlaySoundOnNewMessage })
  }

  const handleShowNotificationOnNewMessageChange = (
    _event: ChangeEvent,
    newShowNotificationOnNewMessage: boolean
  ) => {
    updateUserSettings({
      showNotificationOnNewMessage: newShowNotificationOnNewMessage,
    })
  }

  const handleShowActiveTypingStatusChange = (
    _event: ChangeEvent,
    newShowActiveTypingStatus: boolean
  ) => {
    updateUserSettings({ showActiveTypingStatus: newShowActiveTypingStatus })
  }

  const handleIsEnhancedConnectivityEnabledChange = (
    _event: ChangeEvent,
    newIsEnhancedConnectivityEnabled: boolean
  ) => {
    // Only update if enhanced connectivity is available
    if (isEnhancedConnectivityAvailable) {
      updateUserSettings({
        isEnhancedConnectivityEnabled: newIsEnhancedConnectivityEnabled,
      })
    }
  }

  const handleDeleteSettingsClick = () => {
    setIsDeleteSettingsConfirmDiaglogOpen(true)
  }

  const handleDeleteSettingsCancel = () => {
    setIsDeleteSettingsConfirmDiaglogOpen(false)
  }

  const handleDeleteSettingsConfirm = async () => {
    await persistedStorage.clear()
    window.location.reload()
  }

  const handleExportSettingsClick = async () => {
    try {
      await settings.exportSettings(getUserSettings())
    } catch (e) {
      if (isErrorWithMessage(e)) {
        showAlert(e.message, { severity: 'error' })
      }
    }
  }

  const handleImportSettingsClick = async ([[, file]]: Result[]) => {
    try {
      const userSettings = await settings.importSettings(file)

      await updateUserSettings(userSettings)

      showAlert(t("Profile successfully imported"), { severity: 'success' })
    } catch (e) {
      if (isErrorWithMessage(e)) {
        showAlert(e.message, { severity: 'error' })
      }
    }
  }

  const areNotificationsAvailable = notification.permission === 'granted'

  return (
    <Box sx={{ p: 2, mx: 'auto', maxWidth: theme.breakpoints.values.md }}>
      <Typography
        variant="h2"
        sx={{
          fontSize: theme.typography.h3.fontSize,
          fontWeight: theme.typography.fontWeightMedium,
          mb: 2,
        }}
      >{t("Chat")}</Typography>
      <Paper elevation={3} sx={{ p: 2, mb: 2 }}>
        <Typography>{t("When a message is received in the background:")}</Typography>
        <FormGroup>
          <FormControlLabel
            control={
              <Switch
                checked={playSoundOnNewMessage}
                onChange={handlePlaySoundOnNewMessageChange}
              />
            }
            label={t("Play a sound")}
          />
          <FormControlLabel
            control={
              <Switch
                checked={
                  areNotificationsAvailable && showNotificationOnNewMessage
                }
                onChange={handleShowNotificationOnNewMessageChange}
                disabled={!areNotificationsAvailable}
              />
            }
            label={t("Show a notification")}
          />
        </FormGroup>
        <Typography mt={2}>{t("Select a sound that plays when you receive a message:")}</Typography>
        <SoundSelector disabled={!playSoundOnNewMessage} />
      </Paper>
      <Paper elevation={3} sx={{ p: 2, mb: 2 }}>
        <FormGroup>
          <FormControlLabel
            control={
              <Switch
                checked={showActiveTypingStatus}
                onChange={handleShowActiveTypingStatusChange}
              />
            }
            label={t("Show active typing indicators")}
          />
        </FormGroup>
        <Typography variant="subtitle2">{t("Disabling this will also hide your active typing status from others.")}</Typography>
      </Paper>
      <Divider sx={{ my: 2 }} />
      {isEnhancedConnectivityAvailable && (
        <>
          <Typography
            variant="h2"
            sx={{
              fontSize: theme.typography.h3.fontSize,
              fontWeight: theme.typography.fontWeightMedium,
              mb: 2,
            }}
          >{t("Networking")}</Typography>
          <EnhancedConnectivityControl
            isEnabled={isEnhancedConnectivityEnabled}
            onChange={handleIsEnhancedConnectivityEnabledChange}
            variant="subtitle2"
          />
          <Divider sx={{ my: 2 }} />
        </>
      )}
      <NetworkSettings />
      <Typography
        variant="h2"
        sx={{
          fontSize: theme.typography.h3.fontSize,
          fontWeight: theme.typography.fontWeightMedium,
          mb: 2,
        }}
      >{t("Data")}</Typography>
      <Typography
        variant="h2"
        sx={{
          fontSize: theme.typography.h5.fontSize,
          fontWeight: theme.typography.fontWeightMedium,
          mb: 1.5,
        }}
      >{t("Export profile data")}</Typography>
      <Typography
        variant="body1"
        sx={{
          mb: 2,
        }}
      >{t("Export your Chitchatter profile data so that it can be moved to another browser or device.")}{' '}
        <strong>{t("Be careful not to share the exported data with anyone")}</strong>{t(". It contains your unique verification keys.")}</Typography>
      <Button
        variant="outlined"
        sx={{
          mb: 2,
        }}
        onClick={handleExportSettingsClick}
      >{t("Export profile data")}</Button>
      <Typography
        variant="h2"
        sx={{
          fontSize: theme.typography.h5.fontSize,
          fontWeight: theme.typography.fontWeightMedium,
          mb: 1.5,
        }}
      >{t("Import profile data")}</Typography>
      <Typography
        variant="body1"
        sx={{
          mb: 2,
        }}
      >{t("Import your Chitchatter profile that was previously exported from another browser or device.")}</Typography>
      <FileReaderInput
        {...{
          as: 'text',
          onChange: (_e, results) => {
            handleImportSettingsClick(results)
          },
        }}
      >
        <Button
          color="warning"
          variant="outlined"
          sx={{
            mb: 2,
          }}
        >{t("Import profile data")}</Button>
      </FileReaderInput>
      <Typography
        variant="h2"
        sx={{
          fontSize: theme.typography.h5.fontSize,
          fontWeight: theme.typography.fontWeightMedium,
          mb: 1.5,
        }}
      >{t("Delete all profile data")}</Typography>
      <Typography
        variant="body1"
        sx={{
          mb: 2,
        }}
      >
        <strong>{t("Be careful with this")}</strong>{t(". This will cause your user name to change from")}{' '}
        <strong>
          <PeerNameDisplay
            sx={{
              fontWeight: theme.typography.fontWeightMedium,
            }}
          >
            {userId}
          </PeerNameDisplay>
        </strong>{' '}{t("to a new, randomly-assigned name. It will also reset all of your saved Chitchatter application preferences.")}</Typography>
      <Button
        variant="outlined"
        color="error"
        sx={{
          mb: 2,
        }}
        onClick={handleDeleteSettingsClick}
      >{t("Delete all data and restart")}</Button>
      <ConfirmDialog
        isOpen={isDeleteSettingsConfirmDiaglogOpen}
        onCancel={handleDeleteSettingsCancel}
        onConfirm={handleDeleteSettingsConfirm}
      />
      <Typography
        variant="subtitle2"
        sx={{
          mb: 2,
        }}
      >{t("Chitchatter only stores user preferences and never message content of any kind. This preference data is only stored locally on your device and not a server.")}</Typography>
      <Divider sx={{ my: 2 }} />
    </Box>
  )
}

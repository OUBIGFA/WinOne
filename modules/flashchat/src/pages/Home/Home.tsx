import { t, useLanguage } from 'config/i18n'
import { useContext } from 'react'
import { Link } from 'react-router-dom'

import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import FormControl from '@mui/material/FormControl'
import IconButton from '@mui/material/IconButton'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import { Cached } from '@mui/icons-material'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import styled from '@mui/material/styles/styled'
import GitHubIcon from '@mui/icons-material/GitHub'
import MuiLink from '@mui/material/Link'
import Divider from '@mui/material/Divider'

import Logo from 'img/messages-circle.svg?react'

import { Form, Main } from 'components/Elements'
import { PeerNameDisplay } from 'components/PeerNameDisplay'
import { EnhancedConnectivityControl } from 'components/EnhancedConnectivityControl'
import { SettingsContext } from 'contexts/SettingsContext'
import { routes } from 'config/routes'
import { RoomNameType } from 'core/config/roomNames'

import { isEnhancedConnectivityAvailable } from '../../config/enhancedConnectivity'

import { useHome } from './useHome'
import { EmbedCodeDialog } from './EmbedCodeDialog'
import { CommunityRoomSelector } from './CommunityRoomSelector'

const StyledLogo = styled(Logo)({})

export interface HomeProps {
  userId: string
}

export function Home({ userId }: HomeProps) {
  useLanguage()

  const theme = useTheme()
  const { updateUserSettings, getUserSettings } = useContext(SettingsContext)
  const { isEnhancedConnectivityEnabled } = getUserSettings()
  const {
    roomName,
    roomNameType,
    showEmbedCode,
    handleRoomNameChange,
    handleRoomNameTypeChange,
    regenerateRoomName,
    handleFormSubmit,
    handleJoinPublicRoomClick,
    handleJoinPrivateRoomClick,
    handleGetEmbedCodeClick,
    handleEmbedCodeWindowClose,
    isRoomNameValid,
  } = useHome()

  const handleIsEnhancedConnectivityEnabledChange = (
    _event: React.ChangeEvent<{}>,
    newIsEnhancedConnectivityEnabled: boolean
  ) => {
    updateUserSettings({
      isEnhancedConnectivityEnabled: newIsEnhancedConnectivityEnabled,
    })
  }

  return (
    <Box className="Home">
      <EmbedCodeDialog
        showEmbedCode={showEmbedCode}
        handleEmbedCodeWindowClose={handleEmbedCodeWindowClose}
        roomName={roomName}
      />
      <Main
        sx={{
          maxWidth: theme.breakpoints.values.md,
          mt: 3,
          mx: 'auto',
          px: 2,
          textAlign: 'center',
        }}
      >
        <Link to={routes.ABOUT} aria-label={t("Go to About page")} style={{ color: 'inherit', textDecoration: 'none' }}>
          <StyledLogo
            sx={{
              width: 48,
              height: 48,
              mx: 'auto',
              mb: 1,
            }}
          />
          <Typography variant="h5" sx={{ fontWeight: 500 }}>{t('Flash Chat')}</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>{t('Powered by Chitchatter')}</Typography>
        </Link>
        <Form
          onSubmit={handleFormSubmit}
          sx={{ maxWidth: theme.breakpoints.values.sm, mx: 'auto' }}
        >
          <Typography sx={{ mb: 2 }}>{t("Your username:")}{' '}
            <PeerNameDisplay paragraph={false} sx={{ fontWeight: 500 }}>
              {userId}
            </PeerNameDisplay>
          </Typography>
          <FormControl fullWidth>
            <TextField
              label={t("Room name (generated on your device)")}
              variant="outlined"
              value={roomName}
              onChange={handleRoomNameChange}
              InputProps={{
                endAdornment: (
                  <IconButton
                    aria-label={t("Regenerate room id")}
                    onClick={regenerateRoomName}
                    size="small"
                  >
                    <Cached />
                  </IconButton>
                ),
                sx: { fontSize: { xs: '0.9rem', sm: '1rem' } },
              }}
              size="medium"
            />
          </FormControl>
          <Box sx={{ mt: 2, mb: 2 }}>
            <ToggleButtonGroup
              value={roomNameType}
              exclusive
              onChange={handleRoomNameTypeChange}
              aria-label={t("room name type")}
              size="small"
            >
              <ToggleButton value={RoomNameType.UUID} aria-label={t("UUID")}>{t("UUID")}</ToggleButton>
              <ToggleButton
                value={RoomNameType.PASSPHRASE}
                aria-label={t("Passphrase")}
              >{t("Passphrase")}</ToggleButton>
            </ToggleButtonGroup>
            <Typography variant="body2" sx={{ mt: 1 }}>{t("A room name is the only thing protecting a public room. Anyone who learns it can join. For sensitive conversations, use a private room.")}</Typography>
          </Box>
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'center',
              gap: 1,
              mt: 2,
              flexWrap: 'wrap',
            }}
          >
            <Button
              variant="contained"
              onClick={handleJoinPublicRoomClick}
              sx={{
                marginTop: 2,
              }}
              disabled={!isRoomNameValid}
            >{t("Join public room")}</Button>
            <Button
              variant="contained"
              onClick={handleJoinPrivateRoomClick}
              sx={{
                marginTop: 2,
              }}
              disabled={!isRoomNameValid}
            >{t("Join private room")}</Button>
            <Button
              variant="contained"
              color="secondary"
              onClick={handleGetEmbedCodeClick}
              sx={{
                marginTop: 2,
              }}
              disabled={!isRoomNameValid}
            >{t("Get embed code")}</Button>
          </Box>
        </Form>
      </Main>
      <Box component="section" aria-label={t("Additional options and information")}>
        <Divider sx={{ my: 2 }} />
        <Box maxWidth={theme.breakpoints.values.sm} mx="auto" px={2}>
          <CommunityRoomSelector />
        </Box>
        {isEnhancedConnectivityAvailable && (
          <>
            <Divider sx={{ my: 2 }} />
            <Box maxWidth={theme.breakpoints.values.sm} mx="auto" px={2}>
              <EnhancedConnectivityControl
                isEnabled={isEnhancedConnectivityEnabled}
                onChange={handleIsEnhancedConnectivityEnabledChange}
                showSecondaryColor={true}
              />
            </Box>
          </>
        )}
        <Divider sx={{ my: 2 }} />
        <Box
          sx={{
            maxWidth: theme.breakpoints.values.sm,
            mx: 'auto',
            textAlign: 'center',
            px: 2,
          }}
        >
          <Typography variant="body1">{t("This is a free communication tool that is designed for simplicity, privacy, and security. All interaction between you and your online peers is encrypted. There is no record of your conversation once you all leave.")}</Typography>
        </Box>
        <Box
          component="footer"
          sx={{
            mx: 'auto',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
          }}
        >
          <MuiLink
            href="https://github.com/jeremyckahn/chitchatter"
            target="_blank"
            sx={() => ({
              color: theme.palette.text.primary,
            })}
          >
            <IconButton
              size="large"
              edge="start"
              color="inherit"
              aria-label={t("View source code on GitHub")}
            >
              <GitHubIcon sx={{ fontSize: '2em' }} />
            </IconButton>
          </MuiLink>
          <Typography variant="body1" sx={{ textAlign: 'center', mb: 1 }}>{t("Licensed under")}{' '}
            <MuiLink
              href="https://github.com/jeremyckahn/chitchatter/blob/develop/LICENSE"
              target="_blank"
            >{t("GPL v2")}</MuiLink>{t(". Please")}{' '}
            <MuiLink
              href="https://github.com/jeremyckahn/chitchatter/blob/develop/README.md"
              target="_blank"
            >{t("read the docs")}</MuiLink>
            .
          </Typography>
        </Box>
      </Box>
    </Box>
  )
}

import { t, useLanguage } from 'config/i18n'
import Box from '@mui/material/Box'
import ScreenShare from '@mui/icons-material/ScreenShare'
import StopScreenShare from '@mui/icons-material/StopScreenShare'
import Tooltip from '@mui/material/Tooltip'

import { TrysteroTransport } from 'core/transport/TrysteroTransport'

import { useRoomScreenShare } from './useRoomScreenShare'
import { MediaButton } from './MediaButton'

export interface RoomFileUploadControlsProps {
  peerRoom: TrysteroTransport
}

export function RoomScreenShareControls({
  peerRoom,
}: RoomFileUploadControlsProps) {
  useLanguage()

  const { isSharingScreen, handleScreenShareStart, handleScreenShareStop } =
    useRoomScreenShare({
      peerRoom,
    })

  const handleToggleScreenShareButtonClick = () => {
    if (isSharingScreen) {
      handleScreenShareStop()
    } else {
      handleScreenShareStart()
    }
  }

  if (!window.navigator?.mediaDevices?.getDisplayMedia) {
    return <></>
  }

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
      <Tooltip
        title={
          isSharingScreen ? t('Stop sharing screen') : t('Share screen with room')
        }
      >
        <MediaButton
          isActive={isSharingScreen}
          aria-label={t("share screen")}
          onClick={handleToggleScreenShareButtonClick}
        >
          {isSharingScreen ? <ScreenShare /> : <StopScreenShare />}
        </MediaButton>
      </Tooltip>
    </Box>
  )
}

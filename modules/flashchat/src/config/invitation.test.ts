import { invitationUrl, parseInvitation } from './invitation'

test('invitation replaces only the desktop origin and keeps room/password', () => {
  expect(invitationUrl('http://winone-flashchat.localhost/private/room%20name#secret=abc'))
    .toBe('https://chitchatter.im/private/room%20name#secret=abc')
})

test('pasted official invitation preserves its private room secret', () => {
  expect(parseInvitation('https://chitchatter.im/private/a%20b#secret=abc', 'http://localhost:9876'))
    .toEqual({ roomName: 'a b', path: '/private/a%20b#secret=abc' })
  expect(parseInvitation('https://example.com/private/a#secret=abc')).toBeNull()
})

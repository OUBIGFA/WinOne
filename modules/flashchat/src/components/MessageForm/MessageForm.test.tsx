import { fireEvent, render, screen } from '@testing-library/react'
import { MessageForm } from './MessageForm'

test.each([{ isComposing: true }, { keyCode: 229 }])('Enter confirms composed text without sending (%j)', composing => {
  const send = vi.fn()
  render(<MessageForm onMessageSubmit={send} onMessageChange={() => {}} isMessageSending={false} />)
  const input = screen.getByPlaceholderText('Your message')
  fireEvent.change(input, { target: { value: '中文输入' } })
  fireEvent.keyPress(input, { key: 'Enter', code: 'Enter', charCode: 13, ...composing })
  expect(send).not.toHaveBeenCalled()
  expect(input).toHaveValue('中文输入')
  fireEvent.keyPress(input, { key: 'Enter', code: 'Enter', charCode: 13 })
  expect(send).toHaveBeenCalledWith('中文输入')
  expect(input).toHaveValue('')
})

test('Shift+Enter retains a multiline draft and sending blocks duplicate submission', () => {
  const send = vi.fn()
  const { rerender } = render(<MessageForm onMessageSubmit={send} onMessageChange={() => {}} isMessageSending={false} />)
  const input = screen.getByPlaceholderText('Your message')
  fireEvent.change(input, { target: { value: 'line one\nline two' } })
  fireEvent.keyPress(input, { key: 'Enter', charCode: 13, shiftKey: true })
  expect(send).not.toHaveBeenCalled()
  rerender(<MessageForm onMessageSubmit={send} onMessageChange={() => {}} isMessageSending={true} />)
  fireEvent.keyPress(input, { key: 'Enter', charCode: 13 })
  expect(send).not.toHaveBeenCalled()
  expect(input).toHaveValue('line one\nline two')
})

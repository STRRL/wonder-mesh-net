import { useState, useCallback } from 'react'

interface CopyButtonProps {
  text: string
  label?: string
}

export default function CopyButton({ text, label = 'Copy' }: CopyButtonProps) {
  const [status, setStatus] = useState<'idle' | 'copied' | 'error'>('idle')

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text)
      setStatus('copied')
      setTimeout(() => setStatus('idle'), 2000)
    } catch (err) {
      console.error('Failed to copy text to clipboard:', err)
      setStatus('error')
      setTimeout(() => setStatus('idle'), 2000)
    }
  }, [text])

  const backgroundColor = status === 'copied' ? '#28a745' : status === 'error' ? '#dc3545' : undefined
  const buttonText = status === 'copied' ? 'Copied!' : status === 'error' ? 'Failed' : label

  return (
    <button
      onClick={handleCopy}
      style={{
        backgroundColor,
        minWidth: '80px',
      }}
    >
      {buttonText}
    </button>
  )
}

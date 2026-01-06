import { useState, useCallback } from 'react'

interface CopyButtonProps {
  text: string
  label?: string
}

export default function CopyButton({ text, label = 'Copy' }: CopyButtonProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('copy to clipboard:', err)
    }
  }, [text])

  return (
    <button
      onClick={handleCopy}
      style={{
        backgroundColor: copied ? '#28a745' : undefined,
        minWidth: '80px',
      }}
    >
      {copied ? 'Copied!' : label}
    </button>
  )
}

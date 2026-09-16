import React from "react"
import {Text, StyleSheet} from "react-native"
import type {WidgetProps} from "@/layout/registry/registry.ts"

export type TextConfig = {
    text: string
    fontSize?: number
    color?: string
    align?: "left" | "center" | "right"
}

export function TextWidget({config}: WidgetProps<TextConfig>) {
    return (
        <Text
            style={[
                styles.base,
                config.fontSize !== undefined ? {fontSize: config.fontSize} : null,
                config.color !== undefined ? {color: config.color} : null,
                config.align !== undefined ? {textAlign: config.align} : null,
            ]}
        >
            {config.text}
        </Text>
    )
}

const styles = StyleSheet.create({
    base: {
        flexShrink: 1,
    },
})

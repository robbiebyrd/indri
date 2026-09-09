import {Pressable, StyleSheet, Text, View} from 'react-native'

export type SelectOption = {
    value: string
    label: string
}

export type SelectProps = {
    options: SelectOption[]
    value?: string
    onChange: (value: string) => void
    placeholder?: string
}

// Select is a dependency-free, cross-platform replacement for react-select
// (which is DOM-only and breaks native builds). It renders each option as a
// pressable row and highlights the current selection.
export default function Select({options, value, onChange, placeholder}: SelectProps) {
    return (
        <View style={styles.container}>
            {options.length === 0 && (
                <Text style={styles.placeholder}>{placeholder ?? "No options"}</Text>
            )}
            {options.map((option) => {
                const selected = option.value === value

                return (
                    <Pressable
                        key={option.value}
                        onPress={() => onChange(option.value)}
                        style={[styles.option, selected && styles.optionSelected]}
                    >
                        <Text style={[styles.optionText, selected && styles.optionTextSelected]}>
                            {option.label}
                        </Text>
                    </Pressable>
                )
            })}
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        width: '100%',
        gap: 4,
    },
    placeholder: {
        color: '#888',
        padding: 8,
    },
    option: {
        paddingVertical: 8,
        paddingHorizontal: 12,
        borderWidth: 1,
        borderColor: '#ccc',
        borderRadius: 4,
    },
    optionSelected: {
        borderColor: '#2563eb',
        backgroundColor: '#e0ecff',
    },
    optionText: {
        color: '#222',
    },
    optionTextSelected: {
        color: '#1d4ed8',
        fontWeight: '600',
    },
})

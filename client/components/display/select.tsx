import {Pressable, StyleSheet, Text, View} from 'react-native'

export type SelectOption = {
    value: string
    label: string
}

export type SelectProps = {
    options: readonly SelectOption[]
    value?: string
    onChange: (value: string) => void
    placeholder?: string
}

// Select is a dependency-free, cross-platform replacement for react-select
// (which is DOM-only and breaks native builds). It renders each option as a
// pressable row and highlights the current selection.
export default function Select({options, value, onChange, placeholder}: SelectProps) {
    return (
        <OptionList
            options={options}
            placeholder={placeholder}
            isSelected={(option) => option.value === value}
            onPress={(option) => onChange(option.value)}
        />
    )
}

export type MultiSelectProps = {
    options: readonly SelectOption[]
    // The chosen values IN THE ORDER THEY WERE CHOSEN. This component never
    // reorders them; it only reports which row was pressed.
    values: readonly string[]
    // One row, pressed. Adding or removing it is the caller's decision, so the
    // ordering rules live with whoever owns the value rather than in here.
    onToggle: (value: string) => void
    placeholder?: string
}

// MultiSelect is Select with more than one row lit at a time. It shares Select's
// rows rather than restating them so the two controls cannot drift apart, and
// exists for the same reason Select does: the DOM-only libraries break native.
export function MultiSelect({options, values, onToggle, placeholder}: MultiSelectProps) {
    return (
        <OptionList
            options={options}
            placeholder={placeholder}
            isSelected={(option) => values.includes(option.value)}
            onPress={(option) => onToggle(option.value)}
        />
    )
}

type OptionListProps = {
    options: readonly SelectOption[]
    placeholder?: string
    isSelected: (option: SelectOption) => boolean
    onPress: (option: SelectOption) => void
}

function OptionList({options, placeholder, isSelected, onPress}: OptionListProps) {
    return (
        <View style={styles.container}>
            {options.length === 0 && (
                <Text style={styles.placeholder}>{placeholder ?? "No options"}</Text>
            )}
            {options.map((option) => {
                const selected = isSelected(option)

                return (
                    <Pressable
                        key={option.value}
                        onPress={() => onPress(option)}
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
        // 44pt is the minimum touch target both iOS and Android publish; the
        // padding alone leaves a row short of it on a single-line label.
        minHeight: 44,
        justifyContent: 'center',
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

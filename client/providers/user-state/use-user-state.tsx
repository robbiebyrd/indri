import {useContext} from 'react';
import {UserStateContext} from "@/providers/user-state/user-state-context";

export const useUserState = () => {
    const context = useContext(UserStateContext);
    if (context === undefined) {
        throw new Error('useUserState must be used within a UserStateProvider');
    }
    return context;
};

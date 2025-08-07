import React, {ReactNode, useReducer} from 'react'
import {dataHandler} from "@/providers/user-state/user-state-actions";
import {initialState, UserStateContext} from "@/providers/user-state/user-state-context";

interface UserStateProviderProps {
    children: ReactNode;
}

export const UserStateProvider: React.FC<UserStateProviderProps> = ({children}) => {
    const [userState, dispatch] = useReducer(dataHandler, initialState);

    return (
        <UserStateContext.Provider value={{userState, dispatch}}>
            {children}
        </UserStateContext.Provider>
    );
};
